<?php
// Synthetic records only; no visitors, credentials or camera data.
declare(strict_types=1);
require dirname(__DIR__) . '/public/_app/DownloadLog.php';
require dirname(__DIR__) . '/public/_app/ReportAuth.php';
$count = 0;
function check(bool $ok, string $message): void { global $count; if (!$ok) throw new RuntimeException($message); $count++; }
$dir = sys_get_temp_dir() . '/sadptools-log-test-' . bin2hex(random_bytes(8));
mkdir($dir, 0700); $file = $dir . '/downloads.jsonl'; touch($file);
try {
    $request = new DownloadRequest('macos.dmg', [
        'REMOTE_ADDR'=>'192.0.2.15', 'HTTP_X_FORWARDED_FOR'=>'198.51.100.7',
        'HTTP_USER_AGENT'=>"Synthetic / <script>alert(1)</script>\n\"test\"", 'HTTP_AUTHORIZATION'=>'synthetic-secret',
        'REQUEST_URI'=>'/download/macos.dmg?password=synthetic-secret', 'HTTP_REFERER'=>'https://example.test/?secret=synthetic-secret',
    ]);
    $request->record['version'] = '0.6.0-rc.2'; $request->record['outcome'] = 'sent';
    $request->finish($file); $request->finish($file);
    $rows = iterator_to_array(DownloadLog::records($file));
    check(count($rows) === 1, 'One record per request, including duplicate shutdown calls');
    check($rows[0]['ip'] === '192.0.2.15', 'Direct peer IP; forwarded input is not trusted');
    check($rows[0]['version'] === '0.6.0-rc.2' && $rows[0]['http_status'] === 200, 'Version and status retained');
    check(!str_contains(file_get_contents($file), 'synthetic-secret'), 'No auth, referer or query secrets recorded');
    check(count(file($file)) === 1, 'User-agent newlines cannot inject records');
    check(isset($rows[0]['finished_at'], $rows[0]['duration_ms']), 'Completion time and duration recorded');
    check(DownloadLog::client('Mozilla/5.0 (Windows NT 10.0) Chrome/130.0 Safari/537.36 Edg/130.0') === ['Edge 130.0', 'Windows'], 'Edge detected before Chrome');
    check(DownloadLog::client('Mozilla/5.0 (Macintosh; Intel Mac OS X) Version/18.0 Safari/605.1') === ['Safari 18.0', 'macOS'], 'Safari and Mac detected');
    check(DownloadLog::client('Mozilla/5.0 (X11; Linux x86_64) Firefox/130.0') === ['Firefox 130.0', 'Linux'], 'Firefox and Linux detected');
    check(DownloadLog::client('Unrecognised synthetic client') === ['Unknown', 'Unknown'], 'Unknown client preserved');
    $large = new DownloadRequest('linux.deb', ['HTTP_USER_AGENT'=>str_repeat('x', 5000), 'REMOTE_ADDR'=>'invalid']);
    check(strlen($large->record['user_agent']) === 2048 && $large->record['ip'] === '', 'Untrusted values bounded and validated');
    file_put_contents($file, '{"torn":', FILE_APPEND);
    DownloadLog::append($file, $rows[0]);
    check(count(iterator_to_array(DownloadLog::records($file))) === 2, 'Torn line isolated without losing following records');
    file_put_contents($file, str_repeat('x', 20000) . "\n", FILE_APPEND);
    DownloadLog::append($file, $rows[0]);
    check(count(iterator_to_array(DownloadLog::records($file))) === 3, 'Oversized malformed line skipped');
    if (!function_exists('pcntl_fork')) throw new RuntimeException('pcntl required for concurrent-writer tests.');
    $children = [];
    for ($worker = 0; $worker < 4; $worker++) {
        $pid = pcntl_fork();
        if ($pid === -1) throw new RuntimeException('Cannot start writer test.');
        if ($pid === 0) {
            for ($n = 0; $n < 40; $n++) { $row = $rows[0]; $row['id'] = "$worker-$n"; DownloadLog::append($file, $row); }
            exit(0);
        }
        $children[] = $pid;
    }
    foreach ($children as $pid) { pcntl_waitpid($pid, $status); check(pcntl_wexitstatus($status) === 0, 'Concurrent writer completed'); }
    $all = iterator_to_array(DownloadLog::records($file));
    check(count($all) === 163 && count(array_unique(array_column($all, 'id'))) === 161, 'All concurrent records intact');
    $auth = $dir . '/auth.json';
    file_put_contents($auth, json_encode(['username'=>'fixture', 'password_hash'=>password_hash('synthetic-test-only', PASSWORD_DEFAULT)]));
    check(ReportAuth::allowed(['PHP_AUTH_USER'=>'fixture', 'PHP_AUTH_PW'=>'synthetic-test-only'], $auth), 'Valid report login');
    check(!ReportAuth::allowed([], $auth), 'Anonymous login denied');
    check(!ReportAuth::allowed(['PHP_AUTH_USER'=>'fixture', 'PHP_AUTH_PW'=>'wrong'], $auth), 'Incorrect password denied');
    check(!ReportAuth::allowed(['PHP_AUTH_USER'=>'wrong', 'PHP_AUTH_PW'=>'synthetic-test-only'], $auth), 'Incorrect username denied');
    check(ReportAuth::allowed(['HTTP_AUTHORIZATION'=>'Basic ' . base64_encode('fixture:synthetic-test-only')], $auth), 'CGI header login supported');
    check(!ReportAuth::allowed(['HTTP_AUTHORIZATION'=>'Basic invalid!'], $auth), 'Malformed Basic auth denied');
    unlink($auth);
    try { ReportAuth::allowed([], $auth); throw new LogicException('Missing login must fail closed'); }
    catch (RuntimeException) { check(true, 'Missing login fails closed'); }
} finally { foreach (glob($dir . '/*') as $path) unlink($path); rmdir($dir); }
echo "$count download log and authentication assertions passed\n";
