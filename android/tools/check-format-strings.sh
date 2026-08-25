#!/usr/bin/env bash
#
# Every % in a string resource must be something the formatter can read.
#
# This exists because `%1` shipped where `%1$s` was meant. That is not a
# compile error and not a typo the eye catches: the formatter throws when the
# text is drawn, so the application installs, runs, and closes the moment
# somebody reaches the screen that uses the string.
#
# Android Lint has checks for this and they were tried first. They did not fire
# - proved by putting the original string back and watching the build pass - so
# they were removed rather than left in place looking like protection. Lint
# validates format strings against call sites it recognises, and a Compose
# `stringResource(id, arg)` is not one of them.
#
# So the rule is checked directly, on the resource itself, with no dependency
# on how the string is later used.

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
failed=0

# A valid specifier is %% or % [argument index] [flags] [width] [.precision]
# conversion. Anything else after a % is a mistake.
pattern='%(?!%|(\d+\$)?[-#+ 0,(]*\d*(\.\d+)?[bBhHsScCdoxXeEfgGaAtTn])'

while IFS= read -r file; do
  if matches=$(grep -nP "${pattern}" "${file}"); then
    echo "Unreadable format specifier in ${file#"${root}/"}:"
    echo "${matches}"
    failed=1
  fi
done < <(find "${root}/app/src/main/res" -name 'strings.xml')

if [ "${failed}" -ne 0 ]; then
  echo
  echo "A placeholder must name its argument and its type: %1\$s for text,"
  echo "%1\$d for a number. Bare %1 throws when the text is drawn."
  exit 1
fi

echo "Format strings are readable."
