#!/usr/bin/env bash
set -euo pipefail
WEB_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
SITE_URL="${1:-http://127.0.0.1:8790/}"
VALIDATOR="${SADPTOOLS_SITE_VALIDATOR:-/home/paul/bin/validate-site.sh}"
[[ -f "$VALIDATOR" ]] || { printf 'Missing validator: %s\n' "$VALIDATOR" >&2; exit 1; }
while IFS= read -r -d '' file; do php -l "$file"; done < <(find "$WEB_DIR/public" -name '*.php' -print0)
php "$WEB_DIR/tests/proxy.php"
bash "$VALIDATOR" --mode both "$SITE_URL"
