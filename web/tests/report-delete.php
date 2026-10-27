<?php
declare(strict_types=1);
require dirname(__DIR__) . '/public/_app/ReportDeletion.php';
$count = 0;
function check(bool $ok, string $message): void { global $count; if (!$ok) throw new RuntimeException($message); $count++; }
$dir = sys_get_temp_dir() . '/sadptools-delete-test-' . bin2hex(random_bytes(8)); mkdir($dir, 0700);
$path = $dir . '/downloads.jsonl'; touch($path); chmod($path, 0777);
$base = ['id'=>'keep', 'started_at'=>'2026-01-01T00:00:00Z', 'target'=>'macos.dmg', 'outcome'=>'sent'];
try {
    DownloadLog::append($path, $base);
    for ($n=0; $n<220; $n++) DownloadLog::append($path, array_replace($base, ['id'=>'match-'.$n, 'target'=>'linux.deb']));
    $options = DownloadReport::options(['q'=>'linux']);
    $pending = ReportDeletion::preview($path, $options, ['all_matching'=>'1', 'excluded'=>['match-0']]);
    check($pending['count'] === 219 && !in_array('keep', $pending['ids']), 'All search matches across pages with exclusions');
    $single = ReportDeletion::preview($path, $options, ['all_matching'=>'1', 'single_id'=>'match-1']);
    check($single['ids'] === ['match-1'], 'Individual delete overrides bulk selection');
    check(ReportDeletion::preview($path, $options, ['ids'=>['match-1','match-2']])['count'] === 2, 'Explicit selection');
    foreach ([[], ['ids'=>['keep']], ['ids'=>['../bad']], ['ids'=>['a'=>[]]]] as $post) {
        try { ReportDeletion::preview($path, $options, $post); throw new LogicException('Bad selection accepted'); }
        catch (InvalidArgumentException) { check(true, 'Empty, unrelated or malformed selection denied'); }
    }
    $session = ['delete_pending'=>$pending];
    check(ReportDeletion::consume($session, $pending['token'])['count'] === 219 && !isset($session['delete_pending']), 'Confirmation is consumed once');
    try { ReportDeletion::consume($session, $pending['token']); throw new LogicException('Replay accepted'); }
    catch (InvalidArgumentException) { check(true, 'Replay denied'); }
    $expired = $pending; $expired['created'] -= 601; $session = ['delete_pending'=>$expired];
    try { ReportDeletion::consume($session, $expired['token']); throw new LogicException('Expired confirmation accepted'); }
    catch (InvalidArgumentException) { check(true, 'Expired confirmation denied'); }
    $server = ['HTTP_HOST'=>'sadptools.com', 'HTTP_ORIGIN'=>'https://sadptools.com'];
    check(ReportDeletion::checkCSRF(['csrf'=>'test-token'], $server, 'test-token', true), 'Same-origin CSRF accepted');
    check(!ReportDeletion::checkCSRF([], $server, 'test-token', true), 'Missing CSRF rejected');
    check(!ReportDeletion::checkCSRF(['csrf'=>'wrong'], $server, 'test-token', true), 'Wrong CSRF rejected');
    check(!ReportDeletion::checkCSRF(['csrf'=>'test-token'], array_replace($server, ['HTTP_ORIGIN'=>'https://evil.invalid']), 'test-token', true), 'Cross-origin action rejected');
    // This matching arrival was not part of the confirmed snapshot.
    DownloadLog::append($path, array_replace($base, ['id'=>'new-arrival','target'=>'linux.deb']));
    clearstatcache(); $before = stat($path);
    check(DownloadLog::delete($path, $pending['ids']) === 219, 'Exactly the confirmed records removed');
    $remaining = array_column(iterator_to_array(DownloadLog::records($path)), 'id');
    check($remaining === ['keep', 'match-0', 'new-arrival'], 'Nonmatches, exceptions and later arrivals preserved');
    clearstatcache(); $after = stat($path);
    check($before['ino'] === $after['ino'] && $before['uid'] === $after['uid'] && $before['gid'] === $after['gid'] && ($after['mode'] & 0777) === 0777, 'Inode, owner, group and mode preserved');
    check(!file_exists($path . '.recovery'), 'No recovery copy remains after success');
    file_put_contents($path, "malformed fixture\n", FILE_APPEND);
    check(DownloadLog::delete($path, ['keep']) === 1 && str_contains(file_get_contents($path), 'malformed fixture'), 'Malformed/unselected bytes preserved');
    $original = file_get_contents($path);
    file_put_contents($path . '.recovery', $original); file_put_contents($path, 'interrupted rewrite');
    iterator_to_array(DownloadLog::records($path));
    check(file_get_contents($path) === $original && !file_exists($path . '.recovery'), 'Read recovers an interrupted deletion');
    file_put_contents($path . '.recovery', $original); file_put_contents($path, 'interrupted rewrite');
    DownloadLog::append($path, array_replace($base, ['id'=>'after-recovery']));
    check(str_starts_with(file_get_contents($path), $original), 'Append recovers before writing a new record');
    $beforeInvalidRecovery = file_get_contents($path);
    touch($path . '.recovery');
    try { iterator_to_array(DownloadLog::records($path)); throw new LogicException('Empty recovery accepted'); }
    catch (RuntimeException) { check(file_get_contents($path) === $beforeInvalidRecovery, 'Empty recovery never truncates the log'); }
    unlink($path . '.recovery');
    // Concurrent appends cannot be lost when the original file is rewritten.
    $child = pcntl_fork();
    if ($child === 0) {
        for ($n=0; $n<80; $n++) DownloadLog::append($path, array_replace($base, ['id'=>'parallel-'.$n]));
        exit(0);
    }
    if ($child < 0) throw new RuntimeException('Cannot start writer test');
    DownloadLog::delete($path, ['match-0','new-arrival']);
    pcntl_waitpid($child, $status);
    $remaining = array_column(iterator_to_array(DownloadLog::records($path)), 'id');
    check(pcntl_wexitstatus($status) === 0 && count(array_filter($remaining, fn($id)=>str_starts_with($id, 'parallel-'))) === 80, 'All concurrent appends survive deletion');
    check(!in_array('match-0', $remaining) && in_array('after-recovery', $remaining), 'Only selected IDs removed under concurrency');
} finally { foreach (glob($dir.'/*') as $file) unlink($file); rmdir($dir); }
echo "$count deletion, recovery and concurrency assertions passed\n";
