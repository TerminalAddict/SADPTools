<?php
declare(strict_types=1);

final class ReportAuth
{
    public const FILE = __DIR__ . '/download-report-auth.json';

    public static function allowed(array $server, string $path = self::FILE): bool
    {
        $text = @file_get_contents($path);
        if ($text === false) throw new RuntimeException('Report login is not configured.');
        $config = json_decode($text, true, 8, JSON_THROW_ON_ERROR);
        if (!is_array($config) || !is_string($config['username'] ?? null) ||
            !is_string($config['password_hash'] ?? null) || !password_get_info($config['password_hash'])['algo']) {
            throw new RuntimeException('Report login is not configured.');
        }
        $user = $server['PHP_AUTH_USER'] ?? ''; $password = $server['PHP_AUTH_PW'] ?? '';
        if ($user === '') {
            $header = $server['HTTP_AUTHORIZATION'] ?? $server['REDIRECT_HTTP_AUTHORIZATION'] ?? '';
            if (is_string($header) && strlen($header) < 2048 && preg_match('/^Basic ([A-Za-z0-9+\/=]+)$/iD', $header, $m)) {
                $decoded = base64_decode($m[1], true);
                if ($decoded !== false && str_contains($decoded, ':')) [$user, $password] = explode(':', $decoded, 2);
            }
        }
        if (!is_string($user) || !is_string($password) || strlen($password) > 512) return false;
        // Always verify the password, including for an incorrect username.
        $verified = password_verify($password, $config['password_hash']);
        return hash_equals($config['username'], $user) && $verified;
    }
}
