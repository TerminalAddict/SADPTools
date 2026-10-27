<?php
// Development-only equivalent of the Apache routes; never deployed.
declare(strict_types=1);
$root = dirname(__DIR__) . '/public';
$path = rawurldecode(parse_url($_SERVER['REQUEST_URI'], PHP_URL_PATH));
if (str_contains($path, "\0")) { http_response_code(400); exit; }
if (preg_match('~^/(?:_app|\.git|\.credentials|wordfence-waf\.php|assets/private-downloads(?:/|$))~i', $path)) { http_response_code(403); exit('Forbidden'); }
require $root . '/wordfence-waf.php';
if ($path === '/api/releases') { require $root . '/releases.php'; return true; }
if (preg_match('~^/download/([a-z0-9.-]+)$~D', $path, $m)) { $_GET['target'] = $m[1]; require $root . '/download.php'; return true; }
if ($path === '/') $path = '/index.html';
$resolved = realpath($root . $path);
if ($resolved && (str_starts_with($resolved, $root . '/assets/private-downloads/') || str_starts_with($resolved, $root . '/_app/'))) { http_response_code(403); exit('Forbidden'); }
if (!$resolved || !str_starts_with($resolved, $root . '/') || !is_file($resolved)) { http_response_code(404); require $root . '/404.html'; return true; }
return false;
