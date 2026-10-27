<?php
declare(strict_types=1);
require_once __DIR__ . '/DownloadLog.php';

/** Shared query/order rules for the HTML table and authenticated exports. */
final class DownloadReport
{
    public const PAGE_SIZE = 200;
    public const COLUMNS = ['date'=>'Date / time', 'ip'=>'IP address', 'browser'=>'Browser / OS',
        'version'=>'Version / package', 'transfer'=>'Transfer', 'result'=>'Result', 'bytes'=>'Bytes sent'];
    public const CSV_FIELDS = [
        'id'=>'Request ID', 'started_at'=>'Started (UTC)', 'started_local'=>'Started (Pacific/Auckland)',
        'finished_at'=>'Finished (UTC)', 'ip'=>'IP address', 'browser'=>'Browser', 'os'=>'Operating system',
        'version'=>'Version', 'target'=>'Package', 'filename'=>'Filename', 'transfer'=>'Transfer',
        'range'=>'Requested range', 'outcome'=>'Result', 'http_status'=>'HTTP status',
        'bytes_sent'=>'Bytes sent', 'package_bytes'=>'Package bytes', 'duration_ms'=>'Duration (ms)', 'user_agent'=>'User agent',
    ];

    public static function options(array $query): array
    {
        foreach (['q', 'sort', 'direction', 'page', 'export'] as $key) {
            if (isset($query[$key]) && !is_string($query[$key])) throw new InvalidArgumentException('Invalid report options.');
        }
        $q = trim($query['q'] ?? '');
        if (strlen($q) > 256 || !preg_match('//u', $q)) throw new InvalidArgumentException('Search must be valid text of at most 256 bytes.');
        $sort = $query['sort'] ?? 'date'; $direction = $query['direction'] ?? 'desc';
        $page = $query['page'] ?? '1'; $export = $query['export'] ?? '';
        if (!isset(self::COLUMNS[$sort]) || !in_array($direction, ['asc', 'desc'], true) ||
            !preg_match('/^[1-9][0-9]{0,8}$/D', $page) || !in_array($export, ['', 'jsonl', 'csv'], true)) {
            throw new InvalidArgumentException('Invalid report options.');
        }
        return ['q'=>$q, 'sort'=>$sort, 'direction'=>$direction, 'page'=>(int)$page, 'export'=>$export];
    }

    public static function localTime(mixed $value): string
    {
        if (!is_string($value) || $value === '') return '';
        try { return (new DateTimeImmutable($value))->setTimezone(new DateTimeZone('Pacific/Auckland'))->format('Y-m-d H:i:s T'); }
        catch (Throwable) { return ''; }
    }

    private static function text(array $row, string $key): string
    {
        return isset($row[$key]) && is_scalar($row[$key]) ? (string)$row[$key] : '';
    }

    private static function matches(array $row, array $terms): bool
    {
        if (!$terms) return true;
        $values = [];
        foreach (self::CSV_FIELDS as $key => $_) $values[] = self::text($row, $key);
        $values[] = self::localTime($row['started_at'] ?? null);
        $haystack = implode(' ', $values);
        foreach ($terms as $term) if (!preg_match('~' . preg_quote($term, '~') . '~iu', $haystack)) return false;
        return true;
    }

    private static function compare(array $a, array $b, string $sort): int
    {
        $text = fn(string $key): int => strnatcasecmp(self::text($a, $key), self::text($b, $key));
        return match ($sort) {
            'date' => (strtotime(self::text($a, 'started_at')) ?: 0) <=> (strtotime(self::text($b, 'started_at')) ?: 0),
            'bytes' => (int)($a['bytes_sent'] ?? 0) <=> (int)($b['bytes_sent'] ?? 0),
            'ip' => self::compareIP(self::text($a, 'ip'), self::text($b, 'ip')),
            'version' => version_compare(self::text($a, 'version'), self::text($b, 'version')) ?: $text('target'),
            'browser' => $text('browser') ?: $text('os'),
            'transfer' => $text('transfer'),
            'result' => $text('outcome') ?: ((int)($a['http_status'] ?? 0) <=> (int)($b['http_status'] ?? 0)),
        };
    }

    private static function compareIP(string $a, string $b): int
    {
        $packedA = @inet_pton($a); $packedB = @inet_pton($b);
        if ($packedA !== false && $packedB !== false) return (strlen($packedA) <=> strlen($packedB)) ?: strcmp($packedA, $packedB);
        return strnatcasecmp($a, $b);
    }

    public static function select(string $path, array $options): array
    {
        $terms = preg_split('/\s+/u', $options['q'], -1, PREG_SPLIT_NO_EMPTY);
        $rows = []; $total = 0; $sent = 0; $ranges = 0; $problems = 0;
        foreach (DownloadLog::records($path) as $row) {
            $total++;
            if (($row['outcome'] ?? '') === 'sent' && ($row['transfer'] ?? '') === 'full') $sent++;
            elseif (($row['outcome'] ?? '') === 'sent') $ranges++;
            else $problems++;
            if (self::matches($row, $terms)) $rows[] = ['sequence'=>$total, 'row'=>$row];
        }
        $direction = $options['direction'] === 'asc' ? 1 : -1;
        usort($rows, fn(array $a, array $b): int => $direction *
            (self::compare($a['row'], $b['row'], $options['sort']) ?: ($a['sequence'] <=> $b['sequence'])));
        $matches = count($rows); $pages = max(1, (int)ceil($matches / self::PAGE_SIZE));
        $page = min($options['page'], $pages); $offset = ($page - 1) * self::PAGE_SIZE;
        // Exports include the whole matching result, independent of the viewed page.
        if ($options['export'] === '') $rows = array_slice($rows, $offset, self::PAGE_SIZE);
        return ['rows'=>array_column($rows, 'row'), 'total'=>$total, 'sent'=>$sent, 'ranges'=>$ranges,
            'problems'=>$problems, 'matches'=>$matches, 'page'=>$page, 'pages'=>$pages, 'offset'=>$offset];
    }

    public static function csvRow(array $row): array
    {
        $cells = [];
        foreach (self::CSV_FIELDS as $key => $_) {
            $value = $key === 'started_local' ? self::localTime($row['started_at'] ?? null) : self::text($row, $key);
            // Keep untrusted cells as text on initial spreadsheet import. The JSON
            // export retains raw values for machine processing without this prefix.
            if (preg_match('/^(?:[\x00-\x20]*[=+@\-＝＋＠－]|[\t\r\n])/u', $value)) $value = "'" . $value;
            $cells[] = $value;
        }
        return $cells;
    }
}
