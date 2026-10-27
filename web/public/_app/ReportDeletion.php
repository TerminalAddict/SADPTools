<?php
declare(strict_types=1);
require_once __DIR__ . '/DownloadReport.php';

final class ReportDeletion
{
    public static function startSession(bool $secure): void
    {
        session_name('sadptools_report');
        ini_set('session.use_strict_mode', '1');
        ini_set('session.use_only_cookies', '1');
        session_cache_limiter('');
        session_set_cookie_params(['lifetime'=>0, 'path'=>'/assets/downloads_report.php',
            'secure'=>$secure, 'httponly'=>true, 'samesite'=>'Strict']);
        if (!session_start()) throw new RuntimeException('Report session is unavailable.');
        if (!isset($_SESSION['csrf'])) $_SESSION['csrf'] = bin2hex(random_bytes(32));
    }

    public static function checkCSRF(array $post, array $server, string $token, bool $secure): bool
    {
        if (!is_string($post['csrf'] ?? null) || !hash_equals($token, $post['csrf'])) return false;
        if (($server['HTTP_SEC_FETCH_SITE'] ?? '') === 'cross-site') return false;
        if (isset($server['HTTP_ORIGIN'])) {
            $expected = ($secure ? 'https://' : 'http://') . ($server['HTTP_HOST'] ?? '');
            if ($server['HTTP_ORIGIN'] !== $expected) return false;
        }
        return true;
    }

    private static function ids(mixed $input): array
    {
        if (!is_array($input) || count($input) > DownloadReport::PAGE_SIZE) throw new InvalidArgumentException('Invalid selection.');
        foreach ($input as $id) if (!is_string($id) || !preg_match('/^[A-Za-z0-9._-]{1,100}$/D', $id)) throw new InvalidArgumentException('Invalid selection.');
        return array_fill_keys($input, true);
    }

    /** Capture exact IDs now; later matching arrivals are outside this confirmation. */
    public static function preview(string $path, array $options, array $post): array
    {
        $all = ($post['all_matching'] ?? '') === '1';
        $selected = self::ids($post['ids'] ?? []);
        $excluded = self::ids($post['excluded'] ?? []);
        if (isset($post['single_id'])) { $selected = self::ids([$post['single_id']]); $all = false; }
        if (!$all && !$selected) throw new InvalidArgumentException('Select at least one record to delete.');
        $result = DownloadReport::select($path, array_replace($options, ['export'=>'jsonl']));
        $ids = []; $sample = [];
        foreach ($result['rows'] as $row) {
            $id = $row['id'];
            if ($all ? isset($excluded[$id]) : !isset($selected[$id])) continue;
            self::ids([$id]);
            if (isset($ids[$id])) throw new RuntimeException('Duplicate record identifiers prevent a precise deletion.');
            $ids[$id] = true;
            if (count($sample) < 5) $sample[] = $row;
        }
        if (!$ids) throw new InvalidArgumentException('The selected records are no longer in these search results.');
        return ['ids'=>array_keys($ids), 'count'=>count($ids), 'sample'=>$sample, 'options'=>$options,
            'all'=>$all, 'created'=>time(), 'token'=>bin2hex(random_bytes(24))];
    }

    public static function consume(array &$session, mixed $token): array
    {
        $pending = $session['delete_pending'] ?? null;
        if (!is_array($pending) || !is_string($token) || !hash_equals($pending['token'], $token) ||
            time() - $pending['created'] > 600) {
            throw new InvalidArgumentException('This confirmation has expired or was already used. Select the records again.');
        }
        unset($session['delete_pending']);
        return $pending;
    }
}
