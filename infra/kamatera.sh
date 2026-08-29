#!/usr/bin/env bash
#
# The one place that knows how to talk to this provider.
#
# Sourced, not run. Every call it makes has already been found out the hard
# way, and each one has a comment saying what was wrong with the obvious
# version:
#
#   - the API lives at cloudcli.cloudwm.com, not at the address the customer
#     panel is served from
#   - identifiers travel in the body, not in the path, throughout
#   - an address is found by looking like an address, because the documented
#     field returned nothing against the real answer
#   - removal is judged by the machine being gone, because a terminate call
#     was once accepted and did nothing
#
# It exists because that list was learnt three times. Twice it was written down
# in one workflow and invented again in the next, and the second invention cost
# a run each time. A shared file does not make the knowledge better; it makes
# the second invention impossible without deleting something.
#
# Expects KAMATERA_ACCESS_KEY and KAMATERA_SECRET_KEY in the environment.

KAMATERA_API="${KAMATERA_API:-https://cloudcli.cloudwm.com}"

_kamatera_client() { printf '%s' "${KAMATERA_ACCESS_KEY}" | tr -d '[:space:]'; }
_kamatera_secret() { printf '%s' "${KAMATERA_SECRET_KEY}" | tr -d '[:space:]'; }

# kamatera_servers <file>
#
# Every machine on the account, as the provider lists them.
kamatera_servers() {
    curl -sS \
        -H "AuthClientId: $(_kamatera_client)" \
        -H "AuthSecret: $(_kamatera_secret)" \
        -H "Accept: application/json" \
        "${KAMATERA_API}/service/servers" > "$1"
}

# kamatera_id_of <servers-file> <name>
#
# Empty when there is no such machine, which is a fact rather than a failure:
# a retirement asked twice should say "already gone", not fall over.
kamatera_id_of() {
    jq -r --arg n "$2" \
        'if type == "array" then (.[] | select(.name == $n) | .id) else empty end' \
        "$1" | head -1
}

# kamatera_info <id> <file>
#
# The identifier goes in the body. The path form exists in no documentation
# this project could find and returns nothing useful when guessed at.
kamatera_info() {
    curl -sS -X POST \
        -H "AuthClientId: $(_kamatera_client)" \
        -H "AuthSecret: $(_kamatera_secret)" \
        -H "Content-Type: application/json" -H "Accept: application/json" \
        -d "$(jq -n --arg id "$1" '{id: $id}')" \
        "${KAMATERA_API}/service/server/info" > "$2" 2>/dev/null || true
}

# kamatera_address_of <info-file>
#
# Found by what it looks like rather than by where it sits: the documented path
# returned nothing against the real answer. Private ranges are dropped, because
# a machine usually has one of those as well and it is not the way in.
#
# Prints nothing when there is none, and does not fail: an empty answer is
# something the caller has to decide about, not something to die on.
kamatera_address_of() {
    jq -r '[.. | strings | select(test("^([0-9]{1,3}[.]){3}[0-9]{1,3}$"))]
           | map(select(test("^(10[.]|172[.](1[6-9]|2[0-9]|3[01])[.]|192[.]168[.]|127[.])") | not))
           | unique | .[0] // empty' "$1" 2>/dev/null || true
}

# kamatera_terminate <id>
#
# Two shapes, in this order. The first shape asked here was accepted and did
# nothing: empty body, machine still running, caller satisfied. So the outcome
# is never judged by this function - see kamatera_gone.
kamatera_terminate() {
    local id="$1" shape url payload code
    for shape in body path; do
        case "${shape}" in
            body)
                url="${KAMATERA_API}/service/server/terminate"
                payload=$(jq -n --arg id "${id}" '{id: $id, force: true}')
                ;;
            path)
                url="${KAMATERA_API}/service/server/${id}/terminate"
                payload='{"force": true}'
                ;;
        esac

        code=$(curl -sS -o /tmp/kamatera-terminate.json -w '%{http_code}' -X POST \
            -H "AuthClientId: $(_kamatera_client)" \
            -H "AuthSecret: $(_kamatera_secret)" \
            -H "Content-Type: application/json" -H "Accept: application/json" \
            -d "${payload}" "${url}" || echo "000")

        echo "  ${shape}: HTTP ${code}"
        case "${code}" in 200|201|204) return 0 ;; esac

        # Field names only, never values: a refusal can carry a password.
        jq -r '(if type == "array" then .[0] else . end) | keys_unsorted | join(", ")' \
            /tmp/kamatera-terminate.json 2>/dev/null \
            | sed 's/^/    fields in the refusal: /' || true
    done
    return 1
}

# kamatera_gone <name>
#
# Whether the machine has actually left, asked repeatedly because removal is
# not instant. This is the only thing that decides whether a destroy worked.
kamatera_gone() {
    local name="$1" attempt still
    for attempt in $(seq 1 10); do
        kamatera_servers /tmp/kamatera-after.json
        still=$(jq -r --arg n "${name}" \
            'if type == "array" then ([.[] | select(.name == $n)] | length) else 0 end' \
            /tmp/kamatera-after.json)
        [ "${still}" = "0" ] && return 0
        echo "  still listed, waiting (${attempt}/10)"
        sleep 20
    done
    return 1
}

# kamatera_wait_for_address <name> <attempts> <file>
#
# Waits until a machine has an address, and prints it.
#
# This replaced following the provider's command queue, which answered with
# {error, message} on every poll while the machine was being built perfectly
# well. Two things were wrong with that: the queue endpoint was guessed, and
# the thing being waited for was not the thing that was needed.
#
# What a build actually needs is an address. Asking for it directly removes a
# guess and shortens the chain: the machine is ready when it can be reached,
# not when a queue says a word.
kamatera_wait_for_address() {
    local name="$1" attempts="${2:-40}" out="${3:-/tmp/kamatera-one.json}"
    local attempt id address

    for attempt in $(seq 1 "${attempts}"); do
        kamatera_servers /tmp/kamatera-servers.json
        id=$(kamatera_id_of /tmp/kamatera-servers.json "${name}")

        if [ -n "${id}" ]; then
            kamatera_info "${id}" "${out}"
            address=$(kamatera_address_of "${out}")
            if [ -n "${address}" ]; then
                printf '%s' "${address}"
                return 0
            fi
            echo "  built, waiting for an address (${attempt}/${attempts})" >&2
        else
            echo "  not listed yet (${attempt}/${attempts})" >&2
        fi
        sleep 15
    done
    return 1
}

# kamatera_say_refusal <file>
#
# Every string the provider sent, wherever in the answer it put it, except
# anything whose name suggests a secret.
#
# Printing field names only is what this replaces. A run that fails has to say
# why: the first time this mattered, the reason - the account's active server
# quota was full - arrived by email, to a person, hours later.
kamatera_say_refusal() {
    jq -r '
        [paths(strings) as $p | {k: ($p | map(tostring) | join(".")), v: getpath($p)}]
        | map(select((.k | test("password|secret|key|token"; "i")) | not))
        | map(select((.v | length) > 0))
        | .[:12][] | "  \(.k): \(.v)"' "$1" 2>/dev/null \
    || jq -r '"  fields: " + (keys_unsorted | join(", "))' "$1" 2>/dev/null \
    || echo "  the answer could not be read"
}
