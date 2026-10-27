<?php
declare(strict_types=1);
ini_set('display_errors', '0');
require_once dirname(__DIR__) . '/_app/ReportAuth.php';
require_once dirname(__DIR__) . '/_app/ReportDeletion.php';
header('Cache-Control: private, no-store, max-age=0');
header('Pragma: no-cache');
header('X-Robots-Tag: noindex, nofollow, noarchive');
// Preserve Origin on same-origin form POSTs while withholding external referrers.
header('Referrer-Policy: same-origin');
header('X-Content-Type-Options: nosniff');
$method = $_SERVER['REQUEST_METHOD'] ?? '';
if (!in_array($method, ['GET', 'HEAD', 'POST'], true)) { http_response_code(405); header('Allow: GET, HEAD, POST'); exit; }
$secure = !empty($_SERVER['HTTPS']) && $_SERVER['HTTPS'] !== 'off';
$preview = PHP_SAPI === 'cli-server' && in_array($_SERVER['REMOTE_ADDR'] ?? '', ['127.0.0.1', '::1'], true);
if (!$secure && !$preview) { http_response_code(403); exit('Use the HTTPS report address.'); }
try { $allowed = ReportAuth::allowed($_SERVER); }
catch (Throwable) { http_response_code(503); exit('Report login is not configured.'); }
if (!$allowed) {
    http_response_code(401);
    header('WWW-Authenticate: Basic realm="SADPTools download report", charset="UTF-8"');
    exit; // No records or report content before authentication.
}
if ($method === 'HEAD') exit;
try { $options = DownloadReport::options($method === 'POST' ? $_POST : $_GET); }
catch (InvalidArgumentException $error) { http_response_code(400); exit($error->getMessage()); }
$confirmation = null; $notice = null; $deletionError = null;
if ($method === 'POST' || $options['export'] === '') {
    try { ReportDeletion::startSession($secure); }
    catch (Throwable) { http_response_code(503); exit('The report session is unavailable. Please try again.'); }
    $csrf = $_SESSION['csrf'];
    if ($method === 'POST') {
        if (!ReportDeletion::checkCSRF($_POST, $_SERVER, $csrf, $secure)) {
            session_write_close(); http_response_code(403); exit('Reload the report before submitting this action.');
        }
        try {
            if ($options['export'] !== '') throw new InvalidArgumentException('Deletion requests cannot export records.');
            $action = $_POST['action'] ?? '';
            if ($action === 'preview') {
                $confirmation = ReportDeletion::preview(DownloadLog::FILE, $options, $_POST);
                $_SESSION['delete_pending'] = $confirmation;
            } elseif ($action === 'confirm' || $action === 'cancel') {
                $pending = ReportDeletion::consume($_SESSION, $_POST['intent'] ?? null);
                $options = $pending['options'];
                if ($action === 'confirm') $_SESSION['delete_notice'] = DownloadLog::delete(DownloadLog::FILE, $pending['ids']);
                session_write_close();
                header('Location: /assets/downloads_report.php?' . http_build_query($options, '', '&', PHP_QUERY_RFC3986), true, 303);
                exit;
            } else throw new InvalidArgumentException('Unknown report action.');
        } catch (InvalidArgumentException $error) { http_response_code(400); $deletionError = $error->getMessage(); }
        catch (Throwable) { http_response_code(503); $deletionError = 'Could not complete deletion. Refresh the report before trying again.'; }
    } else {
        $notice = $_SESSION['delete_notice'] ?? null;
        unset($_SESSION['delete_notice']);
    }
    session_write_close();
}
try { $result = DownloadReport::select(DownloadLog::FILE, $options); }
catch (Throwable) { http_response_code(503); exit('Download records are temporarily unavailable.'); }
if ($options['export'] !== '') {
    $csv = $options['export'] === 'csv';
    header('Content-Type: ' . ($csv ? 'text/csv' : 'application/x-ndjson') . '; charset=utf-8');
    header('Content-Disposition: attachment; filename="SADPTools-downloads.' . ($csv ? 'csv' : 'jsonl') . '"');
    $output = fopen('php://output', 'wb');
    if ($csv) {
        fwrite($output, "\xEF\xBB\xBF"); // UTF-8 marker for spreadsheet applications.
        fputcsv($output, array_values(DownloadReport::CSV_FIELDS), ',', '"', '', "\r\n");
    }
    foreach ($result['rows'] as $record) {
        if ($csv) fputcsv($output, DownloadReport::csvRow($record), ',', '"', '', "\r\n");
        else fwrite($output, json_encode($record, JSON_THROW_ON_ERROR | JSON_INVALID_UTF8_SUBSTITUTE) . "\n");
    }
    fclose($output);
    exit;
}
$recent = $result['rows'];
$escape = fn($value): string => htmlspecialchars((string)$value, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');
$url = function (array $changes = []) use ($options, $escape): string {
    $params = array_replace($options, ['page'=>1, 'export'=>''], $changes);
    return $escape('?' . http_build_query($params, '', '&', PHP_QUERY_RFC3986));
};
$hiddenOptions = '';
foreach (['q','sort','direction','page'] as $key) $hiddenOptions .= '<input type="hidden" name="' . $key . '" value="' . $escape($options[$key]) . '">';
$scriptVersion = substr(hash_file('sha256', __DIR__ . '/download-report.js'), 0, 12);
$styleVersion = substr(hash_file('sha256', __DIR__ . '/download-report.css'), 0, 12);
header('Content-Type: text/html; charset=utf-8');
?>
<!doctype html>
<html lang="en-NZ"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow, noarchive"><title>Private download report — SADPTools</title>
<meta name="color-scheme" content="light dark"><link rel="icon" href="/assets/appicon.svg" type="image/svg+xml">
<script src="/assets/theme-init.js"></script><link rel="stylesheet" href="/assets/site.css"><link rel="stylesheet" href="/assets/download-report.css?v=<?= $styleVersion ?>">
<script src="/assets/download-report.js?v=<?= $scriptVersion ?>" defer></script>
</head><body><main class="report">
<?php if ($confirmation): ?>
<section class="report-confirmation" aria-labelledby="delete-heading"><p class="eyebrow">REVIEW DELETION</p><h1 id="delete-heading">Delete <?= $confirmation['count'] ?> download <?= $confirmation['count'] === 1 ? 'record' : 'records' ?>?</h1>
<p>This permanently removes these records from the log and its exports. New records arriving after this confirmation was prepared are excluded.</p>
<p><strong><?= $confirmation['all'] ? 'All selected matches across every page' : 'Selected records' ?></strong> · Search: <?= $escape($options['q'] !== '' ? $options['q'] : 'All records (no filter)') ?></p>
<ul class="delete-sample"><?php foreach ($confirmation['sample'] as $sample): ?><li><?= $escape(DownloadReport::localTime($sample['started_at'] ?? null)) ?> · <?= $escape($sample['ip'] ?? '') ?> · <?= $escape($sample['target'] ?? '') ?></li><?php endforeach; ?></ul>
<?php if ($confirmation['count'] > count($confirmation['sample'])): ?><p>Showing <?= count($confirmation['sample']) ?> of <?= $confirmation['count'] ?> selected records.</p><?php endif; ?>
<form id="confirm-deletion" method="post" action="/assets/downloads_report.php"><input type="hidden" name="csrf" value="<?= $escape($csrf) ?>"><input type="hidden" name="intent" value="<?= $escape($confirmation['token']) ?>"><?= $hiddenOptions ?>
<button class="button button-danger" name="action" value="confirm">Delete <?= $confirmation['count'] ?> <?= $confirmation['count'] === 1 ? 'record' : 'records' ?></button><button class="button button-secondary" name="action" value="cancel">Cancel</button></form></section>
<?php else: ?>
<?php if ($notice !== null): ?><p class="report-notice" role="status">Deleted <?= (int)$notice ?> <?= (int)$notice === 1 ? 'record' : 'records' ?>.</p><?php endif; ?>
<?php if ($deletionError !== null): ?><p class="report-error" role="alert"><?= $escape($deletionError) ?></p><?php endif; ?>
<header class="report-header"><div><p class="eyebrow">SADPTOOLS · PRIVATE REPORT</p><h1>Downloads</h1><p>Times shown in Pacific/Auckland. Raw records use UTC.</p></div><nav aria-label="Report actions"><a class="button" href="<?= $url(['page'=>$result['page']]) ?>">Refresh</a><a id="export-csv" class="button" href="<?= $url(['export'=>'csv']) ?>">Export <?= $options['q'] !== '' ? 'matching ' : '' ?>CSV</a><a id="export-json" class="button" href="<?= $url(['export'=>'jsonl']) ?>">Export <?= $options['q'] !== '' ? 'matching ' : '' ?>JSON</a></nav></header>
<dl class="report-totals"><div><dt>Package requests</dt><dd><?= $result['total'] ?></dd></div><div><dt>Full files sent</dt><dd><?= $result['sent'] ?></dd></div><div><dt>Ranges sent</dt><dd><?= $result['ranges'] ?></dd></div><div><dt>Failed / interrupted</dt><dd><?= $result['problems'] ?></dd></div></dl>
<p>Totals cover all recorded requests. A file sent by the server does not confirm a saved file or installation. Resumes and automated requests are separate entries. HEAD checks, checksum downloads and invalid package names are not counted.</p>
<form class="report-search" method="get" action="/assets/downloads_report.php" role="search">
<div><label for="report-search">Search download records</label><input type="search" id="report-search" name="q" value="<?= $escape($options['q']) ?>" maxlength="256" placeholder="IP, browser, version, package, date…" aria-describedby="search-help" autocomplete="off"></div>
<input type="hidden" name="sort" value="<?= $escape($options['sort']) ?>"><input type="hidden" name="direction" value="<?= $escape($options['direction']) ?>">
<button type="submit" class="button">Search</button><a class="text-link" href="<?= $url(['q'=>'']) ?>">Clear search</a></form>
<p id="search-help">Search the full history. Words match across fields, including user agents and UTC or Auckland dates. Exports include every matching record in the selected order.</p>
<p id="report-count" role="status"><?= number_format($result['matches']) ?> matching requests · Showing <?= $result['matches'] ? $result['offset'] + 1 : 0 ?>–<?= $result['offset'] + count($recent) ?></p>
<?php if (!$recent): ?><p class="report-empty"><?= $result['total'] ? 'No download records match your search.' : 'No downloads have been recorded yet.' ?></p><?php else: ?>
<nav class="report-pagination" aria-label="Download record pages">
<?php if ($result['page'] > 1): ?><a class="text-link" rel="prev" href="<?= $url(['page'=>$result['page']-1]) ?>">← Previous</a><?php endif; ?>
<span>Page <?= $result['page'] ?> of <?= $result['pages'] ?></span>
<?php if ($result['page'] < $result['pages']): ?><a class="text-link" rel="next" href="<?= $url(['page'=>$result['page']+1]) ?>">Next →</a><?php endif; ?></nav>
<form id="record-selection" class="report-selection" method="post" action="/assets/downloads_report.php" data-matches="<?= $result['matches'] ?>">
<input type="hidden" name="csrf" value="<?= $escape($csrf) ?>"><input type="hidden" name="action" value="preview"><?= $hiddenOptions ?>
<label><input type="checkbox" name="all_matching" value="1" id="select-matching"> Select all <?= number_format($result['matches']) ?> matching records (every page)</label>
<div class="selection-actions"><button id="delete-selected" class="button button-danger" type="submit">Delete selected</button><button id="clear-selection" class="button button-secondary" type="button" hidden>Clear selection</button><span id="selection-status" role="status">Choose rows below, or select all matches.</span></div><div id="excluded-records"></div>
<noscript><p>“Select all matching” overrides individual checkboxes. Uncheck it to delete specific rows.</p></noscript></form>
<div class="report-table-wrap" role="region" aria-label="Download records" tabindex="0"><table><caption>Download records — click a column heading to sort</caption><thead><tr><th scope="col" class="selection-cell"><input type="checkbox" id="select-page" aria-label="Select all records on this page" disabled></th>
<?php foreach (DownloadReport::COLUMNS as $key => $label): $active = $options['sort'] === $key; $nextDirection = $active && $options['direction'] === 'asc' ? 'desc' : 'asc'; ?>
<th scope="col" aria-sort="<?= $active ? ($options['direction'] === 'asc' ? 'ascending' : 'descending') : 'none' ?>"><a data-sort="<?= $key ?>" href="<?= $url(['sort'=>$key, 'direction'=>$nextDirection]) ?>" aria-label="<?= $label ?>: sort <?= $nextDirection === 'asc' ? 'ascending' : 'descending' ?>"><?= $label ?> <span aria-hidden="true"><?= $active ? ($options['direction'] === 'asc' ? '↑' : '↓') : '↕' ?></span></a></th>
<?php endforeach; ?><th scope="col">Actions</th></tr></thead><tbody>
<?php foreach ($recent as $row): ?><tr>
<td class="selection-cell"><input type="checkbox" name="ids[]" value="<?= $escape($row['id']) ?>" form="record-selection" data-record-checkbox aria-label="Select record from <?= $escape(DownloadReport::localTime($row['started_at'] ?? null)) ?> for <?= $escape($row['ip'] ?? '') ?>"></td>
<td class="date-cell"><?= $escape(DownloadReport::localTime($row['started_at'] ?? null)) ?></td><td><?= $escape($row['ip'] ?? '') ?></td>
<td><?= $escape($row['browser'] ?? 'Unknown') ?><br><?= $escape($row['os'] ?? 'Unknown') ?><details><summary>User agent</summary><p><?= $escape($row['user_agent'] ?? '') ?></p></details></td>
<td><?= $escape($row['version'] ?? 'Unresolved') ?><br><strong><?= $escape($row['target'] ?? '') ?></strong><small><?= $escape($row['filename'] ?? '') ?></small></td>
<td><?= $escape($row['transfer'] ?? '') ?><?php if (!empty($row['range'])): ?><small><?= $escape($row['range']) ?></small><?php endif; ?></td>
<td><?= $escape($row['outcome'] ?? '') ?><small>HTTP <?= $escape($row['http_status'] ?? '') ?> · <?= $escape($row['duration_ms'] ?? '') ?> ms</small></td>
<td class="bytes-cell"><?= number_format((int)($row['bytes_sent'] ?? 0)) ?></td>
<td class="action-cell"><button class="delete-row" type="submit" name="single_id" value="<?= $escape($row['id']) ?>" form="record-selection" aria-label="Delete record from <?= $escape(DownloadReport::localTime($row['started_at'] ?? null)) ?> for <?= $escape($row['ip'] ?? '') ?>">Delete</button></td>
</tr><?php endforeach; ?></tbody></table></div><?php endif; ?>
<p class="report-footnote">Browser and OS labels are inferred from the supplied user agent and can be inaccurate. IP addresses are the server’s direct connection address; forwarded headers are not trusted. Use a private browser window for this report and close it when finished to discard the browser’s remembered login.</p>
<?php endif; ?></main></body></html>
