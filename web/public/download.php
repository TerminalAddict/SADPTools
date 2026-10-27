<?php
declare(strict_types=1);
require_once __DIR__ . '/_app/ReleaseProxy.php';
require_once __DIR__ . '/_app/DownloadLog.php';
ini_set('display_errors', '0');
ini_set('zlib.output_compression', '0');
set_time_limit(240);
header_remove('X-Powered-By');
header('X-Content-Type-Options: nosniff');
header('X-Robots-Tag: noindex');
$method = $_SERVER['REQUEST_METHOD'];
if (!in_array($method, ['GET', 'HEAD'], true)) { http_response_code(405); header('Allow: GET, HEAD'); exit; }
$key = $_GET['target'] ?? '';
if (!is_string($key) || (!isset(ReleaseProxy::TARGETS[$key]) && $key !== 'checksums.txt')) {
    http_response_code(404); header('Content-Type: text/plain; charset=utf-8'); echo 'Download not found.'; exit;
}
$audit = null;
if ($method === 'GET' && $key !== 'checksums.txt') {
    try {
        $audit = new DownloadRequest($key, $_SERVER);
        register_shutdown_function($audit->finish(...));
        ignore_user_abort(true);
    } catch (Throwable) { error_log('SADPTools download logging could not be started.'); }
}
try {
    $proxy = new ReleaseProxy(); $release = $proxy->release()['release'];
    if ($key === 'checksums.txt') {
        header('Content-Type: text/plain; charset=utf-8');
        header('Content-Disposition: attachment; filename="SADPTools-SHA256SUMS.txt"');
        header('Cache-Control: no-cache');
        if ($method !== 'HEAD') foreach ($release['assets'] as $asset) echo $asset['sha256'] . '  ' . $asset['name'] . "\n";
        exit;
    }
    $asset = $release['assets'][$key]; $size = $asset['size']; $etag = '"' . $asset['sha256'] . '"';
    if ($audit) {
        $audit->record['version'] = $release['version'];
        $audit->record['filename'] = $asset['name'];
        $audit->record['package_bytes'] = $size;
    }
    $range = $_SERVER['HTTP_RANGE'] ?? '';
    if (isset($_SERVER['HTTP_IF_RANGE']) && $_SERVER['HTTP_IF_RANGE'] !== $etag) $range = '';
    $start = 0; $end = $size - 1;
    if ($audit) $audit->record['transfer'] = $range !== '' ? 'range' : 'full';
    if ($range !== '' && $method !== 'HEAD') {
        try { [$start, $end] = ReleaseProxy::range($range, $size); }
        catch (InvalidArgumentException) {
            if ($audit) $audit->record['outcome'] = 'invalid_range';
            http_response_code(416); header('Content-Range: bytes */' . $size); exit;
        }
    }
    // Finish the upstream transfer and checksum before sending any executable bytes.
    $file = $method === 'HEAD' ? null : $proxy->assetFile($asset);
    $stream = $file === null ? null : fopen($file, 'rb');
    if ($file !== null && $stream === false) throw new RuntimeException('Download unavailable.');
    header('Content-Type: ' . $asset['type']);
    header('Content-Disposition: attachment; filename="' . $asset['name'] . '"');
    header('Accept-Ranges: bytes'); header('ETag: ' . $etag);
    header('Cache-Control: no-cache'); header('X-Download-SHA256: ' . $asset['sha256']);
    if ($range !== '' && $method !== 'HEAD') { http_response_code(206); header("Content-Range: bytes $start-$end/$size"); }
    header('Content-Length: ' . ($end - $start + 1));
    if ($method === 'HEAD') exit;
    while (ob_get_level() > 0) ob_end_clean();
    fseek($stream, $start); $remaining = $end - $start + 1;
    while ($remaining > 0 && !feof($stream) && !connection_aborted()) {
        $chunk = fread($stream, min(65536, $remaining));
        if ($chunk === false || $chunk === '') break;
        echo $chunk; $remaining -= strlen($chunk);
        if ($audit) $audit->record['bytes_sent'] += strlen($chunk);
        flush();
    }
    fclose($stream);
    if ($audit) $audit->record['outcome'] = $remaining === 0 && !connection_aborted() ? 'sent' : 'interrupted';
} catch (Throwable) {
    if ($audit) $audit->record['outcome'] = 'failed';
    if (!headers_sent()) {
        http_response_code(503); header('Content-Type: text/plain; charset=utf-8');
        header('Cache-Control: no-store'); header('Retry-After: 60');
        if ($method !== 'HEAD') echo 'We could not prepare this download. Please wait a moment and try again.';
    }
}
