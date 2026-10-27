<?php
declare(strict_types=1);

/** Locked JSON Lines with explicit deletion. No auth headers or URL queries. */
final class DownloadLog
{
    public const FILE = __DIR__ . '/../assets/private-downloads/downloads.jsonl';

    public static function append(string $path, array $record): void
    {
        $line = json_encode($record, JSON_THROW_ON_ERROR | JSON_INVALID_UTF8_SUBSTITUTE | JSON_UNESCAPED_SLASHES) . "\n";
        // Provisioned by Paul: never recreate a missing file as the Apache user.
        if (is_link($path)) throw new RuntimeException('Log is unavailable.');
        $file = @fopen($path, 'r+b');
        if ($file === false) throw new RuntimeException('Log is unavailable.');
        try {
            if (!flock($file, LOCK_EX)) throw new RuntimeException('Log is busy.');
            self::recover($path, $file);
            fseek($file, 0, SEEK_END);
            $size = ftell($file);
            if ($size > 0) {
                fseek($file, -1, SEEK_END);
                // Isolate a torn final record after a killed process or full disk.
                if (fread($file, 1) !== "\n") $line = "\n" . $line;
            }
            fseek($file, 0, SEEK_END);
            $offset = 0;
            while ($offset < strlen($line)) {
                $written = fwrite($file, substr($line, $offset));
                if ($written === false || $written === 0) throw new RuntimeException('Log write failed.');
                $offset += $written;
            }
            if (!fflush($file)) throw new RuntimeException('Log flush failed.');
        } finally { flock($file, LOCK_UN); fclose($file); }
    }

    /** Read under the same lock as append/delete; never observe a partial rewrite. */
    public static function records(string $path): Generator
    {
        if (is_link($path)) throw new RuntimeException('Log is unavailable.');
        $file = @fopen($path, 'r+b');
        if ($file === false) throw new RuntimeException('Log is unavailable.');
        try {
            if (!flock($file, LOCK_EX)) throw new RuntimeException('Log is busy.');
            self::recover($path, $file);
            rewind($file);
            $size = fstat($file)['size'];
            while (ftell($file) < $size) {
                $line = fgets($file, min(16385, $size - ftell($file) + 1));
                if ($line === false) break;
                if (!str_ends_with($line, "\n")) {
                    // Discard incomplete or oversized lines, never parse their tail.
                    while (ftell($file) < $size && !str_ends_with($line, "\n")) {
                        $line = fgets($file, min(16385, $size - ftell($file) + 1));
                        if ($line === false) break;
                    }
                    continue;
                }
                try { $row = json_decode($line, true, 16, JSON_THROW_ON_ERROR); }
                catch (JsonException) { continue; }
                if (is_array($row) && isset($row['id'], $row['started_at'], $row['target'], $row['outcome'])) yield $row;
            }
        } finally { flock($file, LOCK_UN); fclose($file); }
    }

    private static function flush($file): void
    {
        if (!fflush($file) || !fsync($file)) throw new RuntimeException('Log flush failed.');
    }

    private static function replaceContents($destination, $source): void
    {
        rewind($source); rewind($destination);
        $size = fstat($source)['size'];
        // Keep the original inode, Paul ownership and permissions.
        if (!ftruncate($destination, 0) || stream_copy_to_stream($source, $destination) !== $size) {
            throw new RuntimeException('Log rewrite failed.');
        }
        self::flush($destination);
    }

    private static function syncDirectory(string $path): void
    {
        $directory = @fopen(dirname($path), 'r');
        if ($directory === false) throw new RuntimeException('Cannot sync log directory.');
        try { if (!fsync($directory)) throw new RuntimeException('Cannot sync log directory.'); }
        finally { fclose($directory); }
    }

    /** A durable before-image rolls back an interrupted/uncommitted deletion. */
    private static function recover(string $path, $file): void
    {
        $recovery = $path . '.recovery';
        clearstatcache(true, $recovery);
        if (is_link($recovery)) throw new RuntimeException('Log recovery is unavailable.');
        if (!file_exists($recovery)) return;
        if (!is_file($recovery) || filesize($recovery) === 0) throw new RuntimeException('Log recovery is unavailable.');
        $saved = @fopen($recovery, 'rb');
        if ($saved === false) throw new RuntimeException('Log recovery is unavailable.');
        try {
            self::replaceContents($file, $saved);
            if (!unlink($recovery)) throw new RuntimeException('Log recovery could not finish.');
            self::syncDirectory($path);
        } finally { fclose($saved); }
    }

    /** Delete only a previously confirmed ID snapshot, retaining all other bytes. */
    public static function delete(string $path, array $ids): int
    {
        $wanted = array_fill_keys($ids, true);
        if (!$wanted) return 0;
        if (is_link($path)) throw new RuntimeException('Log is unavailable.');
        $file = @fopen($path, 'r+b');
        if ($file === false) throw new RuntimeException('Log is unavailable.');
        $kept = null; $backup = null;
        try {
            if (!flock($file, LOCK_EX)) throw new RuntimeException('Log is busy.');
            self::recover($path, $file); rewind($file);
            $kept = tmpfile();
            if ($kept === false) throw new RuntimeException('Cannot prepare deletion.');
            $removed = 0;
            while (($line = fgets($file)) !== false) {
                try { $row = json_decode($line, true, 16, JSON_THROW_ON_ERROR); }
                catch (JsonException) { $row = null; }
                if (is_array($row) && is_string($row['id'] ?? null) && isset($wanted[$row['id']]) &&
                    isset($row['started_at'], $row['target'], $row['outcome']) && str_ends_with($line, "\n")) {
                    $removed++;
                } elseif (fwrite($kept, $line) !== strlen($line)) throw new RuntimeException('Cannot prepare deletion.');
            }
            if (!feof($file)) throw new RuntimeException('Cannot read records for deletion.');
            if (!$removed) return 0;
            self::flush($kept);
            $backup = tempnam(dirname($path), '.download-recovery-');
            if ($backup === false) throw new RuntimeException('Cannot prepare recovery.');
            $saved = fopen($backup, 'w+b');
            if ($saved === false) throw new RuntimeException('Cannot prepare recovery.');
            try {
                rewind($file);
                if (stream_copy_to_stream($file, $saved) !== fstat($file)['size']) throw new RuntimeException('Cannot save recovery.');
                self::flush($saved);
            } finally { fclose($saved); }
            // The private directory is HTTP-denied and ignored by Git/deployment.
            if (!rename($backup, $path . '.recovery')) throw new RuntimeException('Cannot save recovery.');
            $backup = null;
            try {
                self::syncDirectory($path);
                self::replaceContents($file, $kept);
                if (!unlink($path . '.recovery')) throw new RuntimeException('Cannot finish deletion.');
                self::syncDirectory($path);
            } catch (Throwable $error) {
                self::recover($path, $file);
                throw $error;
            }
            return $removed;
        } finally {
            if (is_resource($kept)) fclose($kept);
            if (is_string($backup) && is_file($backup)) unlink($backup);
            flock($file, LOCK_UN); fclose($file);
        }
    }

    public static function client(string $agent): array
    {
        $browser = 'Unknown'; $os = 'Unknown';
        foreach (['Edge'=>'Edg(?:e|A|iOS)?', 'Opera'=>'(?:OPR|Opera)', 'Firefox'=>'(?:Firefox|FxiOS)', 'Chrome'=>'(?:Chrome|CriOS)', 'Safari'=>'Version', 'curl'=>'curl', 'Python'=>'Python-urllib'] as $name => $pattern) {
            if (preg_match('~' . $pattern . '/([0-9.]+)~i', $agent, $m)) { $browser = $name . ' ' . $m[1]; break; }
        }
        if (preg_match('/iPhone|iPad|iPod/i', $agent)) $os = 'iOS / iPadOS';
        elseif (stripos($agent, 'Android') !== false) $os = 'Android';
        elseif (stripos($agent, 'Windows') !== false) $os = 'Windows';
        elseif (preg_match('/Macintosh|Mac OS X/i', $agent)) $os = 'macOS';
        elseif (stripos($agent, 'Linux') !== false) $os = 'Linux';
        return [$browser, $os];
    }
}

/** One row per valid package GET, written even on shutdown/connection failure. */
final class DownloadRequest
{
    private float $started;
    private bool $saved = false;
    public array $record;

    public function __construct(string $target, array $server)
    {
        $this->started = microtime(true);
        $agent = substr((string)($server['HTTP_USER_AGENT'] ?? ''), 0, 2048);
        [$browser, $os] = DownloadLog::client($agent);
        $ip = (string)($server['REMOTE_ADDR'] ?? '');
        $this->record = [
            'id'=>bin2hex(random_bytes(12)), 'started_at'=>gmdate('c'),
            'ip'=>filter_var($ip, FILTER_VALIDATE_IP) ? $ip : '',
            'browser'=>$browser, 'os'=>$os, 'user_agent'=>$agent,
            'target'=>$target, 'version'=>null, 'filename'=>null,
            'package_bytes'=>null, 'range'=>substr((string)($server['HTTP_RANGE'] ?? ''), 0, 128),
            'transfer'=>'full', 'bytes_sent'=>0, 'outcome'=>'interrupted',
        ];
    }

    public function finish(string $path = DownloadLog::FILE): void
    {
        if ($this->saved) return;
        $this->saved = true;
        $this->record['finished_at'] = gmdate('c');
        $this->record['duration_ms'] = (int)round((microtime(true) - $this->started) * 1000);
        $this->record['http_status'] = http_response_code() ?: 200;
        if (connection_aborted()) $this->record['outcome'] = 'interrupted';
        try { DownloadLog::append($path, $this->record); }
        catch (Throwable) { error_log('SADPTools download log could not be written.'); }
    }
}
