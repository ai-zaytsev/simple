#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
tool="${root}/sites/official/tools/update_release_manifest.py"
work=$(mktemp -d)
trap 'rm -rf "${work}"' EXIT

empty="${work}/empty.json"
one="${work}/one.json"
two="${work}/two.json"
: > "${empty}"
printf 'first apk\n' > "${work}/simple-vpn-0.1.0.apk"
printf 'second apk\n' > "${work}/simple-vpn-0.1.1.apk"
printf 'gap apk\n' > "${work}/simple-vpn-0.1.2.apk"

python3 "${tool}" \
  --manifest "${empty}" --output "${one}" \
  --version-name 0.1.0 --version-code 7 \
  --published-at 2026-08-29T20:00:00Z \
  --file "${work}/simple-vpn-0.1.0.apk" \
  --sha256 "$(sha256sum "${work}/simple-vpn-0.1.0.apk" | cut -d' ' -f1)" \
  --certificate-sha256 "$(printf 'a%.0s' {1..64})" \
  --commit "$(printf 'b%.0s' {1..40})"

python3 "${tool}" \
  --manifest "${one}" --output "${two}" \
  --version-name 0.1.1 --version-code 8 \
  --published-at 2026-08-29T21:00:00Z \
  --file "${work}/simple-vpn-0.1.1.apk" \
  --sha256 "$(sha256sum "${work}/simple-vpn-0.1.1.apk" | cut -d' ' -f1)" \
  --certificate-sha256 "$(printf 'a%.0s' {1..64})" \
  --commit "$(printf 'c%.0s' {1..40})"

python3 - "${two}" <<'PY'
import json, pathlib, sys
manifest = json.loads(pathlib.Path(sys.argv[1]).read_text())
assert manifest["latest"] == "0.1.1"
assert [release["versionCode"] for release in manifest["releases"]] == [8, 7]
assert manifest["releases"][1]["url"].endswith("/0.1.0/simple-vpn-0.1.0.apk")
PY

if python3 "${tool}" --manifest "${two}" --output "${work}/bad.json" \
  --version-name 0.1.1 --version-code 9 --published-at 2026-08-29T22:00:00Z \
  --file "${work}/simple-vpn-0.1.1.apk" \
  --sha256 "$(printf 'd%.0s' {1..64})" --certificate-sha256 "$(printf 'a%.0s' {1..64})" \
  --commit "$(printf 'e%.0s' {1..40})" >/dev/null 2>&1; then
  echo "duplicate versionName was accepted"
  exit 1
fi

if python3 "${tool}" --manifest "${two}" --output "${work}/bad.json" \
  --version-name 0.1.2 --version-code 10 --published-at 2026-08-29T22:00:00Z \
  --file "${work}/simple-vpn-0.1.2.apk" \
  --sha256 "$(printf 'd%.0s' {1..64})" --certificate-sha256 "$(printf 'a%.0s' {1..64})" \
  --commit "$(printf 'e%.0s' {1..40})" >/dev/null 2>&1; then
  echo "a versionCode gap was accepted"
  exit 1
fi

if python3 "${tool}" --manifest "${two}" --output "${work}/bad.json" \
  --version-name 0.1.2 --version-code 9 --published-at 2026-08-29T22:00:00Z \
  --file "${work}/simple-vpn-0.1.2.apk" \
  --sha256 "$(printf 'd%.0s' {1..64})" --certificate-sha256 "$(printf 'f%.0s' {1..64})" \
  --commit "$(printf 'e%.0s' {1..40})" >/dev/null 2>&1; then
  echo "a different signing certificate was accepted"
  exit 1
fi

echo "ok: publication is monotonic, immutable and keeps old links"
