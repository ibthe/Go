#!/usr/bin/env bash
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BIN="${TMPDIR:-/tmp}/go-proxy-audit"
REPORT="${PROXY_AUDIT_REPORT:-proxy-audit-report.txt}"

go build -o "$BIN" ./cmd/go

proxies=(
  "primary-ip|https://23.237.103.44:8443"
  "primary-host|https://ff.s1.redirtry.top:8443"
  "backup-ip-1|https://162.244.81.74:8443"
  "backup-ip-2|https://162.244.81.207:8443"
  "youtube-ip|https://23.237.103.46:8443"
  "youtube-host|https://ff.y1.redirtry.top:8443"
  "google-play|https://de26.staticvaultcdn.xyz:443"
)

: > "$REPORT"
printf 'Go proxy endpoint audit\nTarget: example.com:443\nChecks: DNS/TCP, TLS certificate (TLS 1.2+), HTTP CONNECT without credentials\n\n' | tee -a "$REPORT"

passed=0
failed=0
for entry in "${proxies[@]}"; do
  label="${entry%%|*}"
  endpoint="${entry#*|}"
  printf '%-16s %-48s ' "$label" "${endpoint#https://}" | tee -a "$REPORT"
  if output="$("$BIN" check-proxy -endpoint "$endpoint" -target example.com:443 -timeout 12s 2>&1)"; then
    printf 'PASS  %s\n' "$output" | tee -a "$REPORT"
    passed=$((passed + 1))
  else
    # Keep the diagnostic, but never print URL userinfo if a future list adds it.
    printf 'FAIL  %s\n' "$output" | tee -a "$REPORT"
    failed=$((failed + 1))
  fi
done

printf '\nSummary: %s passed, %s failed\n' "$passed" "$failed" | tee -a "$REPORT"
printf 'A PASS confirms only TLS + unauthenticated CONNECT to example.com:443, not UDP, DNS routing, anonymity, ownership, or permission to use the endpoint.\n' | tee -a "$REPORT"

# The audit report is useful even when some candidate endpoints fail.
