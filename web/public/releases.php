<?php
declare(strict_types=1);
require_once __DIR__ . '/_app/ReleaseProxy.php';
header('Content-Type: application/json; charset=utf-8');
header('X-Content-Type-Options: nosniff');
if (!in_array($_SERVER['REQUEST_METHOD'], ['GET', 'HEAD'], true)) { http_response_code(405); header('Allow: GET, HEAD'); exit; }
try {
    $proxy = new ReleaseProxy();
    $data = $proxy->publicMetadata($proxy->release());
    header('Cache-Control: public, max-age=60');
    if ($_SERVER['REQUEST_METHOD'] !== 'HEAD') echo json_encode($data, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES);
} catch (Throwable) {
    http_response_code(503); header('Retry-After: 60'); header('Cache-Control: no-store');
    if ($_SERVER['REQUEST_METHOD'] !== 'HEAD') echo '{"error":"Downloads are temporarily unavailable. Please try again shortly."}';
}
