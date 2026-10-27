<?php
// This host's vhost requires this legacy prepend path. No WordPress or
// Wordfence is installed here. Use it only to set this site's response defaults.
declare(strict_types=1);
ini_set('display_errors', '0');
ini_set('log_errors', '1');
if (PHP_SAPI !== 'cli') {
    header_remove('X-Powered-By');
    header('X-Content-Type-Options: nosniff');
    header('Referrer-Policy: strict-origin-when-cross-origin');
}
