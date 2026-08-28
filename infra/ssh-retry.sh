#!/usr/bin/env bash
#
# Runs one ssh or scp command, and tries again when the connection is dropped
# rather than refused.
#
# The Control Plane host drops connections occasionally: three times in one
# afternoon, from GitHub runners and from a laptop, while the machine itself
# sat idle with no load, no memory pressure, no rate limit and nothing in its
# own logs. Whatever is between us and it is not reliable, and an operation
# that takes eight steps over ssh should not be decided by the least reliable
# second of the day.
#
# Retried only for a dropped connection - exit 255, which ssh uses for its own
# failures. A command that ran and failed on the far side keeps its exit code,
# because running it twice is how a half-finished change becomes two.
#
# Input for the remote command is named by SSH_STDIN rather than read from this
# script's own standard input. A first version read stdin and buffered it, and
# hung the moment it was called somewhere stdin was an open pipe nobody was
# going to close. Explicit is also retryable: a heredoc consumed on the first
# attempt would make the second one send nothing and report success.

set -uo pipefail

ATTEMPTS="${SSH_ATTEMPTS:-4}"
INPUT="${SSH_STDIN:-/dev/null}"

# scp fails the same way for the same reason, so it is retried by the same
# rule. Both use 255 for "the connection failed" and anything else for "the
# far side answered".
PROGRAM="${SSH_PROGRAM:-ssh}"

if [ ! -r "${INPUT}" ]; then
  echo "SSH_STDIN names ${INPUT}, which cannot be read." >&2
  exit 2
fi

attempt=1
while :; do
  "${PROGRAM}" "$@" < "${INPUT}"
  code=$?

  [ "${code}" -eq 0 ] && exit 0

  # 255 is ssh's own: the connection failed, so the command may not have run
  # at all. Anything else came back from the far side and is an answer.
  if [ "${code}" -ne 255 ] || [ "${attempt}" -ge "${ATTEMPTS}" ]; then
    exit "${code}"
  fi

  echo "  connection dropped, trying again (${attempt}/${ATTEMPTS})" >&2
  attempt=$((attempt + 1))
  sleep $((attempt * 5))
done
