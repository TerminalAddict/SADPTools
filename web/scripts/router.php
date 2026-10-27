<?php
// Development-only equivalent of the Apache routes; never deployed.
declare(strict_types=1);
$root = dirname(__DIR__) . '/public';
$path = parse_url($_SERVER['REQUEST_URI'], PHP_URL_PATH);
if (preg_match('~^/(?:_app|\.git|\.credentials|wordfence-waf\.php)~', $path)) { http_response_code(403); exit('Forbidden'); }
require $root . '/wordfence-waf.php';
if ($path === '/api/releases') { require $root . '/releases.php'; return true; }
if (preg_match('~^/download/([a-z0-9.-]+)$~D', $path, $m)) { $_GET['target'] = $m[1]; require $root . '/download.php'; return true; }
if ($path === '/') $path = '/index.html';
$resolved = realpath($root . rawurldecode($path));
if (!$resolved || !str_starts_with($resolved, $root . '/') || !is_file($resolved)) { http_response_code(404); require $root . '/404.html'; return true; }
return false;
