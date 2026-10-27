<?php
// CLI-only provisioning. Input arrives over stdin; no passwords in command args.
declare(strict_types=1);
if (PHP_SAPI !== 'cli') exit(1);
$input = json_decode(stream_get_contents(STDIN), true, 8, JSON_THROW_ON_ERROR);
$root = $input['document_root']; $auth = $input['auth'];
$directory = $root . '/assets/private-downloads';
$authPath = $root . '/_app/download-report-auth.json';
if (!str_contains((string)file_get_contents($directory . '/.htaccess'), 'Require all denied') ||
    !str_contains((string)file_get_contents($root . '/.htaccess'), '^assets/private-downloads')) {
    throw new RuntimeException('Deploy the HTTP access guards before provisioning records.');
}
if (is_file($authPath) && json_decode((string)file_get_contents($authPath), true) !== $auth) {
    throw new RuntimeException('Existing report login differs. Reuse its private credentials file; do not overwrite it.');
}
$log = $directory . '/downloads.jsonl';
foreach ([$directory, $log, $authPath] as $path) {
    if (is_link($path) || (file_exists($path) && function_exists('posix_geteuid') && fileowner($path) !== posix_geteuid())) {
        throw new RuntimeException('Unexpected file ownership or symbolic link.');
    }
}
if (!is_file($log)) {
    $file = fopen($log, 'x');
    if ($file === false) throw new RuntimeException('Could not create download records.');
    fclose($file);
}
if (!chmod($directory, 0777) || !chmod($log, 0777)) throw new RuntimeException('Could not set requested permissions.');
if (!is_file($authPath) && file_put_contents($authPath, json_encode($auth, JSON_THROW_ON_ERROR) . "\n", LOCK_EX) === false) {
    throw new RuntimeException('Could not configure the report login.');
}
chmod($authPath, 0644);
echo "Download records provisioned; existing records preserved. Directory and log: mode 777.\n";
