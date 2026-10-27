<?php
declare(strict_types=1);
require dirname(__DIR__) . '/public/_app/DownloadReport.php';
$count = 0;
function check(bool $ok, string $message): void { global $count; if (!$ok) throw new RuntimeException($message); $count++; }
$path = tempnam(sys_get_temp_dir(), 'sadptools-report-query-');
try {
    $base = ['id'=>'old', 'started_at'=>'2026-01-01T12:30:00+00:00', 'target'=>'macos.dmg', 'outcome'=>'sent', 'transfer'=>'full',
        'browser'=>'Safari', 'os'=>'macOS', 'ip'=>'192.0.2.2', 'version'=>'0.6.0-rc.2', 'bytes_sent'=>2];
    DownloadLog::append($path, $base);
    for ($i=0; $i<220; $i++) DownloadLog::append($path, array_replace($base, [
        'id'=>'new-'.$i, 'started_at'=>'2026-10-27T12:00:00+00:00', 'target'=>'windows-setup.exe',
        'browser'=>'Chrome', 'os'=>'Windows', 'ip'=>'192.0.2.10', 'version'=>'0.6.0-rc.10', 'bytes_sent'=>100,
    ]));
    $select = fn(array $query): array => DownloadReport::select($path, DownloadReport::options($query));
    $all = $select([]);
    check(count($all['rows']) === 200 && $all['matches'] === 221 && $all['pages'] === 2, 'Full history with bounded page size');
    check($all['rows'][0]['id'] === 'new-219', 'Newest first with deterministic equal-time order');
    check($select(['page'=>'2'])['rows'][20]['id'] === 'old', 'Older records available on page two');
    check($select(['page'=>'999'])['page'] === 2, 'Out-of-range page clamped');
    check($select(['q'=>'SAFARI macos'])['rows'][0]['id'] === 'old', 'Case-insensitive multi-field search beyond latest 200');
    check($select(['q'=>'2026-01-02 NZDT'])['matches'] === 1, 'Auckland date is searchable');
    check($select(['q'=>'2026-01-01'])['matches'] === 1, 'UTC date is searchable');
    check($select(['q'=>'no-such-value'])['matches'] === 0, 'No results stays empty');
    check($select(['q'=>'[.*'])['matches'] === 0, 'Query is literal, not a regular expression');
    check($select(['sort'=>'ip','direction'=>'asc'])['rows'][0]['ip'] === '192.0.2.2', 'IP address octets sorted numerically');
    check($select(['sort'=>'version','direction'=>'asc'])['rows'][0]['version'] === '0.6.0-rc.2', 'RC sequence sorted numerically');
    check($select(['sort'=>'bytes','direction'=>'asc'])['rows'][0]['bytes_sent'] === 2, 'Byte counts sorted numerically');
    check($select(['sort'=>'bytes','direction'=>'desc'])['rows'][0]['bytes_sent'] === 100, 'Descending numeric order');
    check(count($select(['export'=>'csv','page'=>'2'])['rows']) === 221, 'Export spans every matching page');
    check(count($select(['export'=>'csv','q'=>'Safari'])['rows']) === 1, 'Export honours search');
    foreach ([['q'=>[]], ['sort'=>'injected'], ['direction'=>'sideways'], ['page'=>'-1'], ['export'=>'php'], ['q'=>str_repeat('a',257)]] as $query) {
        try { DownloadReport::options($query); throw new LogicException('Bad options accepted'); }
        catch (InvalidArgumentException) { check(true, 'Malformed options rejected'); }
    }
    foreach (['=1+1', '+1', '-1', '@SUM(1)', "\t=1", "\r=1", " \t=1", '＝1+1', '＠SUM(1)'] as $payload) {
        $row = DownloadReport::csvRow(['user_agent'=>$payload]);
        check(end($row) === "'".$payload, 'Formula-like agent prefixed as text');
    }
    $row = DownloadReport::csvRow(['user_agent'=>"Normal, \"quoted\" agent\nsecond line", 'bytes_sent'=>1234]);
    $stream = fopen('php://temp', 'w+b'); fputcsv($stream, $row, ',', '"', '', "\r\n"); rewind($stream);
    check(fgetcsv($stream, null, ',', '"', '') === $row, 'CSV round-trip preserves commas, quotes and newlines'); fclose($stream);
} finally { unlink($path); }
echo "$count report query, sorting and CSV assertions passed\n";
