<?php
declare(strict_types=1);

/** Release downloads stay on this origin. No caller-supplied upstream URL. */
final class ReleaseProxy
{
    private const API = 'https://api.github.com/repos/TerminalAddict/SADPTools/releases?per_page=100';
    private const PREFIX = 'https://github.com/TerminalAddict/SADPTools/releases/download/';
    private const TTL = 300;
    private const STALE = 86400;
    private const MAX_ASSET = 536870912;
    public const TARGETS = [
        'windows-setup.exe' => ['SADPTools-%s-Setup-windows-amd64.exe', 'application/vnd.microsoft.portable-executable'],
        'windows-portable.zip' => ['SADPTools-%s-standalone-windows-amd64.zip', 'application/zip'],
        'linux.deb' => ['SADPTools-%s-linux-amd64.deb', 'application/vnd.debian.binary-package'],
        'linux.rpm' => ['SADPTools-%s-linux-x86_64.rpm', 'application/x-rpm'],
        'linux.tar.gz' => ['SADPTools-%s-manual-linux-amd64.tar.gz', 'application/gzip'],
    ];
    private string $cache;
    private Closure $transport;

    public function __construct(?string $cache = null, ?Closure $transport = null)
    {
        $uid = function_exists('posix_geteuid') ? (string)posix_geteuid() : substr(hash('sha256', __DIR__), 0, 12);
        $this->cache = $cache ?? sys_get_temp_dir() . '/sadptools-downloads-' . $uid;
        if (is_link($this->cache) || (!is_dir($this->cache) && !mkdir($this->cache, 0700, true))) {
            throw new RuntimeException('Download cache is unavailable.');
        }
        chmod($this->cache, 0700);
        $this->transport = $transport ?? self::request(...);
    }

    private function readCache(): ?array
    {
        $path = $this->cache . '/release.json';
        if (!is_file($path)) return null;
        try {
            $value = json_decode((string)file_get_contents($path), true, 64, JSON_THROW_ON_ERROR);
            return is_array($value) && isset($value['release'], $value['fetchedAt']) ? $value : null;
        } catch (Throwable) { return null; }
    }

    private function writeCache(array $value): void
    {
        $temp = tempnam($this->cache, 'meta-');
        if ($temp === false) throw new RuntimeException('Cache write failed.');
        try {
            chmod($temp, 0600);
            if (file_put_contents($temp, json_encode($value, JSON_THROW_ON_ERROR)) === false || !rename($temp, $this->cache . '/release.json')) {
                throw new RuntimeException('Cache write failed.');
            }
        } finally { if (is_file($temp)) unlink($temp); }
    }

    public function release(): array
    {
        $cached = $this->readCache();
        $now = time();
        if ($cached && $now - $cached['fetchedAt'] < self::TTL) return $cached;
        $lock = fopen($this->cache . '/metadata.lock', 'c');
        if ($lock === false) throw new RuntimeException('Release metadata is busy.');
        try {
            if (!flock($lock, LOCK_EX | LOCK_NB)) {
                if ($cached && $now - $cached['fetchedAt'] <= self::STALE) return $cached;
                throw new RuntimeException('Release metadata is refreshing.');
            }
            $cached = $this->readCache();
            if ($cached && $now - $cached['fetchedAt'] < self::TTL) return $cached;
            $failure = $this->cache . '/refresh-failed';
            if (is_file($failure) && $now - filemtime($failure) < 60) {
                if ($cached && $now - $cached['fetchedAt'] <= self::STALE) return $cached;
                throw new RuntimeException('Release metadata is temporarily unavailable.');
            }
            try {
                $body = ($this->transport)(self::API, 8388608, null);
                $rows = json_decode($body, true, 64, JSON_THROW_ON_ERROR);
                if (!is_array($rows) || !array_is_list($rows)) throw new RuntimeException('Invalid release catalogue.');
                $release = self::selectRelease($rows);
                $checksums = ($this->transport)($release['checksumsURL'], 65536, null);
                $release['assets'] = self::verifyChecksums($release['assets'], $checksums);
                unset($release['checksumsURL']);
                $cached = ['fetchedAt' => $now, 'release' => $release];
                $this->writeCache($cached);
                if (is_file($failure)) unlink($failure);
                $this->prune();
                return $cached;
            } catch (Throwable $error) {
                touch($failure); chmod($failure, 0600);
                error_log('SADPTools release catalogue refresh failed (' . get_class($error) . ').');
                if ($cached && $now - $cached['fetchedAt'] <= self::STALE) return $cached;
                throw new RuntimeException('Release metadata is temporarily unavailable.');
            }
        } finally { flock($lock, LOCK_UN); fclose($lock); }
    }

    public static function selectRelease(array $rows): array
    {
        $eligible = [];
        foreach ($rows as $row) {
            if (!is_array($row) || ($row['draft'] ?? true) || empty($row['published_at'])) continue;
            if (!preg_match('/^v(\d+\.\d+\.\d+(?:-(?:beta|rc)\.[1-9]\d*)?)$/D', $row['tag_name'] ?? '', $match)) continue;
            $row['_version'] = $match[1]; $eligible[] = $row;
        }
        usort($eligible, fn(array $a, array $b): int => version_compare($b['_version'], $a['_version']));
        if (!$eligible) throw new RuntimeException('No published release is available.');
        $row = $eligible[0]; $version = $row['_version'];
        $assets = [];
        foreach ($row['assets'] ?? [] as $asset) {
            if (($asset['state'] ?? '') !== 'uploaded') continue;
            $name = $asset['name'] ?? '';
            if (!is_string($name) || !preg_match('/^[A-Za-z0-9._-]+$/D', $name) || isset($assets[$name])) throw new RuntimeException('Invalid release asset.');
            $expected = self::PREFIX . rawurlencode($row['tag_name']) . '/' . rawurlencode($name);
            if (($asset['browser_download_url'] ?? '') !== $expected) throw new RuntimeException('Unexpected release location.');
            $size = $asset['size'] ?? 0;
            if (!is_int($size) || $size < 1 || $size > self::MAX_ASSET) throw new RuntimeException('Invalid release size.');
            $assets[$name] = ['name' => $name, 'size' => $size, 'url' => $expected, 'digest' => $asset['digest'] ?? null];
        }
        $selected = [];
        foreach (self::TARGETS as $key => [$pattern, $type]) {
            $name = sprintf($pattern, $version);
            if (!isset($assets[$name])) throw new RuntimeException('The newest release is incomplete.');
            $selected[$key] = $assets[$name] + ['type' => $type];
        }
        if (!isset($assets['SHA256SUMS']) || $assets['SHA256SUMS']['size'] > 65536) throw new RuntimeException('Release checksums are unavailable.');
        return ['version' => $version, 'publishedAt' => $row['published_at'], 'prerelease' => str_contains($version, '-'), 'assets' => $selected, 'checksumsURL' => $assets['SHA256SUMS']['url']];
    }

    public static function verifyChecksums(array $assets, string $text): array
    {
        $hashes = [];
        foreach (preg_split('/\r?\n/', trim($text)) as $line) {
            if (!preg_match('/^([a-fA-F0-9]{64}) [ *]([A-Za-z0-9._-]+)$/D', $line, $match)) throw new RuntimeException('Invalid checksum manifest.');
            if (isset($hashes[$match[2]])) throw new RuntimeException('Duplicate checksum.');
            $hashes[$match[2]] = strtolower($match[1]);
        }
        foreach ($assets as &$asset) {
            if (!isset($hashes[$asset['name']])) throw new RuntimeException('Missing checksum.');
            $asset['sha256'] = $hashes[$asset['name']];
            if ($asset['digest'] !== null && $asset['digest'] !== 'sha256:' . $asset['sha256']) throw new RuntimeException('Conflicting release checksum.');
            unset($asset['digest']);
        }
        return $assets;
    }

    public function publicMetadata(array $cached): array
    {
        $release = $cached['release']; $assets = [];
        foreach ($release['assets'] as $key => $asset) {
            $assets[$key] = ['name' => $asset['name'], 'size' => $asset['size'], 'sha256' => $asset['sha256'], 'download' => '/download/' . $key];
        }
        return ['version' => $release['version'], 'prerelease' => $release['prerelease'], 'publishedAt' => $release['publishedAt'], 'checkedAt' => gmdate('c', $cached['fetchedAt']), 'stale' => time() - $cached['fetchedAt'] >= self::TTL, 'assets' => $assets];
    }

    public function assetFile(array $asset): string
    {
        $path = $this->cache . '/' . $asset['sha256'] . '.bin';
        $lock = fopen($path . '.lock', 'c');
        if ($lock === false || !flock($lock, LOCK_EX | LOCK_NB)) {
            if (is_resource($lock)) fclose($lock);
            throw new RuntimeException('This download is being prepared. Try again shortly.');
        }
        try {
            if (is_file($path) && filesize($path) === $asset['size'] && hash_equals($asset['sha256'], hash_file('sha256', $path))) {
                touch($path); return $path;
            }
            if (is_file($path)) unlink($path);
            $temp = tempnam($this->cache, 'download-');
            if ($temp === false) throw new RuntimeException('Cannot prepare download.');
            chmod($temp, 0600);
            try {
                ($this->transport)($asset['url'], $asset['size'], $temp);
                clearstatcache(true, $temp);
                if (filesize($temp) !== $asset['size'] || !hash_equals($asset['sha256'], hash_file('sha256', $temp))) throw new RuntimeException('Download integrity check failed.');
                if (!rename($temp, $path)) throw new RuntimeException('Cannot cache download.');
                return $path;
            } finally { if (is_file($temp)) unlink($temp); }
        } finally { flock($lock, LOCK_UN); fclose($lock); }
    }

    private function prune(): void
    {
        // Never remove a file while another request is preparing it; streams opened
        // before unlink remain valid. Keep active/old downloads for seven days.
        foreach (glob($this->cache . '/*.bin') ?: [] as $file) {
            if (filemtime($file) > time() - 604800) continue;
            $lock = fopen($file . '.lock', 'c');
            if ($lock && flock($lock, LOCK_EX | LOCK_NB)) {
                if (filemtime($file) <= time() - 604800) unlink($file);
                flock($lock, LOCK_UN);
            }
            if ($lock) fclose($lock);
        }
    }

    public static function approvedURL(string $url): bool
    {
        $p = parse_url($url);
        if (!$p || ($p['scheme'] ?? '') !== 'https' || isset($p['user']) || isset($p['pass']) || isset($p['fragment']) || (isset($p['port']) && $p['port'] !== 443)) return false;
        $host = $p['host'] ?? '';
        if ($host === 'api.github.com') return str_starts_with($url, self::API);
        if ($host === 'github.com') return str_starts_with($url, self::PREFIX);
        return in_array($host, ['release-assets.githubusercontent.com', 'objects.githubusercontent.com'], true);
    }

    private static function request(string $url, int $limit, ?string $destination): string
    {
        for ($redirect = 0; $redirect <= 4; $redirect++) {
            if (!self::approvedURL($url)) throw new RuntimeException('Unapproved download destination.');
            $body = ''; $location = ''; $bytes = 0;
            $stream = $destination !== null ? fopen($destination, 'wb') : null;
            if ($destination !== null && $stream === false) throw new RuntimeException('Cannot write download.');
            $curl = curl_init($url);
            curl_setopt_array($curl, [
                CURLOPT_FOLLOWLOCATION => false, CURLOPT_PROTOCOLS => CURLPROTO_HTTPS,
                CURLOPT_CONNECTTIMEOUT => 10, CURLOPT_TIMEOUT => $destination === null ? 20 : 180,
                CURLOPT_LOW_SPEED_LIMIT => 1024, CURLOPT_LOW_SPEED_TIME => 30,
                CURLOPT_SSL_VERIFYHOST => 2, CURLOPT_SSL_VERIFYPEER => true,
                CURLOPT_USERAGENT => 'SADPTools-Website/1.0',
                CURLOPT_HTTPHEADER => ['Accept: ' . (str_contains($url, 'api.github.com/') ? 'application/vnd.github+json' : 'application/octet-stream'), 'X-GitHub-Api-Version: 2022-11-28'],
                CURLOPT_HEADERFUNCTION => static function ($handle, string $line) use (&$location): int {
                    if (strncasecmp($line, 'Location:', 9) === 0) $location = trim(substr($line, 9));
                    return strlen($line);
                },
                CURLOPT_WRITEFUNCTION => static function ($handle, string $chunk) use (&$bytes, &$body, $stream, $limit): int {
                    $bytes += strlen($chunk);
                    if ($bytes > $limit) return 0;
                    if ($stream !== null) return fwrite($stream, $chunk);
                    $body .= $chunk; return strlen($chunk);
                },
            ]);
            try {
                $ok = curl_exec($curl);
                $status = curl_getinfo($curl, CURLINFO_RESPONSE_CODE);
                if ($ok === false) throw new RuntimeException('Upstream transfer failed.');
            } finally { if (is_resource($stream)) fclose($stream); }
            if (in_array($status, [301, 302, 303, 307, 308], true) && $location !== '') { $url = $location; continue; }
            if ($status !== 200) throw new RuntimeException('Upstream response unavailable.');
            return $body;
        }
        throw new RuntimeException('Too many upstream redirects.');
    }

    /** Inclusive byte bounds. Only a single range is supported. */
    public static function range(string $header, int $size): array
    {
        if (!preg_match('/^bytes=(\d*)-(\d*)$/D', $header, $m) || ($m[1] === '' && $m[2] === '')) throw new InvalidArgumentException('Invalid range.');
        if ($m[1] === '') {
            $suffix = (int)$m[2]; if ($suffix < 1) throw new InvalidArgumentException('Invalid range.');
            return [max(0, $size - $suffix), $size - 1];
        }
        $start = (int)$m[1]; $end = $m[2] === '' ? $size - 1 : min((int)$m[2], $size - 1);
        if ($start >= $size || $start > $end) throw new InvalidArgumentException('Invalid range.');
        return [$start, $end];
    }
}
