#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
site="${root}/sites/official"

for path in index.html 404.html styles.css app.js nginx.conf releases.example.json; do
  test -s "${site}/${path}" || { echo "missing site file: ${path}"; exit 1; }
done

python3 - "${site}" <<'PY'
import html.parser, json, pathlib, sys

site = pathlib.Path(sys.argv[1])

class Page(html.parser.HTMLParser):
    def __init__(self):
        super().__init__()
        self.ids = set()
        self.download = False
        self.archive = False
        self.title = False
    def handle_starttag(self, tag, attrs):
        values = dict(attrs)
        if values.get("id"):
            self.ids.add(values["id"])
        self.download |= "data-download" in values
        self.archive |= "data-version-list" in values
        self.title |= tag == "title"

parser = Page()
parser.feed((site / "index.html").read_text(encoding="utf-8"))
assert parser.title, "page has no title"
assert parser.download, "page has no latest download control"
assert parser.archive, "page has no archive surface"
assert {"download", "versions"}.issubset(parser.ids), "required anchors are absent"

manifest = json.loads((site / "releases.example.json").read_text(encoding="utf-8"))
assert manifest["schema"] == 1
assert manifest["latest"] == manifest["releases"][0]["versionName"]
PY

if grep -RInE '(src|href)="https?://' "${site}"/*.html "${site}"/*.css "${site}"/*.js; then
  echo "official site must not depend on third-party assets"
  exit 1
fi

if command -v node >/dev/null 2>&1; then
  node --check "${site}/app.js"
fi

grep -q 'location = /download/latest.apk' "${site}/nginx.conf"
grep -q 'location \^~ /download/releases/' "${site}/nginx.conf"
grep -q '/apk/releases.json' "${site}/nginx.conf"

bash "${site}/tools/test-publish.sh"
echo "ok: official site has a latest download, archive and no external assets"
