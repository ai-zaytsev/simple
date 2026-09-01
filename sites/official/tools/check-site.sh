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
        self.install_guide = False
        self.title = False
    def handle_starttag(self, tag, attrs):
        values = dict(attrs)
        if values.get("id"):
            self.ids.add(values["id"])
        self.download |= "data-download" in values
        self.archive |= "data-version-list" in values
        self.install_guide |= tag == "details" and "data-install-guide" in values
        self.title |= tag == "title"

parser = Page()
parser.feed((site / "index.html").read_text(encoding="utf-8"))
assert parser.title, "page has no title"
assert parser.download, "page has no latest download control"
assert parser.archive, "page has no archive surface"
assert parser.install_guide, "page has no expandable Android install guide"
assert {"download", "install", "versions"}.issubset(parser.ids), "required anchors are absent"

page = (site / "index.html").read_text(encoding="utf-8")
for phrase in (
    "Установка неизвестных приложений",
    "Неизвестные источники",
    "Samsung Galaxy",
    "Автоблокировка",
    "снова запретите",
):
    assert phrase in page, f"install guidance is missing: {phrase}"

script = (site / "app.js").read_text(encoding="utf-8")
assert script.count('const manifestUrl = "/releases.json"') == 1
assert script.count('download.href = "/download/latest.apk"') == 1

styles = (site / "styles.css").read_text(encoding="utf-8")
assert "@media (max-width: 850px)" in styles, "tablet layout is absent"
assert "@media (max-width: 600px)" in styles, "phone layout is absent"
assert "min-width: 0" in styles, "page must not force horizontal overflow"

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
bash -n "${root}/.github/scripts/deploy-apk-site-content.sh"
grep -q 'runner_ip}/32' "${root}/.github/scripts/deploy-apk-site-content.sh"
grep -q 'trap finish EXIT' "${root}/.github/scripts/deploy-apk-site-content.sh"
if grep -q 'port_range.*22' "${root}/infra/terraform/site.tf"; then
  echo "site-1 must not keep SSH open in its permanent Terraform firewall"
  exit 1
fi
echo "ok: one responsive page has latest, archive, Android/Samsung guidance and no external assets"
