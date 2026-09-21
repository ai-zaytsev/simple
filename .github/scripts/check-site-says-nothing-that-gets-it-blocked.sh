#!/usr/bin/env bash
#
# The public site never says what it is for.
#
# simple-vpn.download was blocked in Russia while sitting behind Cloudflare -
# the origin was never exposed, so nothing was found by address. What was
# matched was the name and the page, and the page said "Скачайте Simple VPN"
# above a paragraph about which traffic goes through the tunnel.
#
# A replacement domain costs money and can only be spent once: the first crawl
# that finds the same words spends it. So the words are refused here rather
# than remembered, because whoever adds one back will be writing something
# true and helpful at the time.
#
# This is not a claim that the product is something else. It is a claim about
# what a download page has to say out loud, which is: how to install it.
#
# The patterns list whole words in each case instead of using grep -i or a
# character class. Neither works here: grep -i folds ASCII only in this C
# locale, and a class like [Вв] is read byte by byte, so it matches almost
# any Cyrillic text at all. The first version of this file used -i and passed
# a page reading "Обход блокировок"; the second flagged every line on the
# site. Only running both against forgeries found either.
set -euo pipefail

SITE="sites/official"
failed=0

# Latin stems, where -i is safe.
latin=("vpn" "proxy")

# Cyrillic stems, spelled out in the cases they realistically appear in.
cyrillic=(
    "(впн|Впн|ВПН)"
    "(обход|Обход|ОБХОД)"
    "(цензур|Цензур|ЦЕНЗУР)"
    "(роскомнадзор|Роскомнадзор|РОСКОМНАДЗОР)"
    "(прокси|Прокси|ПРОКСИ)"
    "(анонимайзер|Анонимайзер|АНОНИМАЙЗЕР)"
    "(разблокир|Разблокир|РАЗБЛОКИР)"
    "(блокиров|Блокиров|БЛОКИРОВ)"
)

# Samsung named a phone setting "Автоблокировка", and the install guide has to
# use that word or the instruction cannot be followed. Excused by that exact
# stem and nothing wider: "блокировка" on its own stays refused.
allowed="(автоблокировк|Автоблокировк|АВТОБЛОКИРОВК)"

report() {
    local file="$1" what="$2" hits="$3"
    echo "${file} says ${what}:"
    printf '%s\n' "${hits}" | sed 's/^/  /'
    failed=1
}

for file in "${SITE}"/index.html "${SITE}"/404.html "${SITE}"/app.js "${SITE}"/styles.css; do
    [ -f "${file}" ] || continue

    for stem in "${latin[@]}"; do
        hits=$(grep -inE "${stem}" "${file}" | grep -vE "${allowed}" || true)
        [ -z "${hits}" ] || report "${file}" "\"${stem}\"" "${hits}"
    done

    for stem in "${cyrillic[@]}"; do
        hits=$(grep -nE "${stem}" "${file}" | grep -vE "${allowed}" || true)
        [ -z "${hits}" ] || report "${file}" "${stem}" "${hits}"
    done

    # The blocked domain carries the word inside itself.
    hits=$(grep -nF 'simple-vpn.download' "${file}" || true)
    [ -z "${hits}" ] || report "${file}" "the blocked domain" "${hits}"
done

if [ "${failed}" -ne 0 ]; then
    echo
    echo "A domain can be spent once. The page that gets crawled decides."
    exit 1
fi

echo "ok: the site says nothing that got the last domain blocked"
