#!/usr/bin/env bash
#
# Stops this machine accepting a password.
#
# Every machine in this fleet was created with password authentication on and
# root allowed to use it, with a password the provider generated. Two nodes had
# taken 26,584 and 20,612 failed password attempts from 149 and 179 different
# addresses, more than half of them against root, since the day they were made.
# That is not a risk of a break-in; it is a break-in being attempted
# continuously, against an account that could accept one.
#
# Keys only afterwards. Nothing in this project has ever used a password to
# reach a machine - the deploy key, the workflows and the operator all use
# keys - so this removes a door nobody walks through and everybody knocks on.
#
# Refuses to run when it cannot see a key to leave behind. Locking everybody
# out of a node carrying people is worse than the thing being fixed.

set -euo pipefail

DROPIN=/etc/ssh/sshd_config.d/10-simple-vpn-keys-only.conf

keys=0
for file in /root/.ssh/authorized_keys /home/*/.ssh/authorized_keys; do
  [ -s "${file}" ] || continue
  keys=$((keys + $(grep -cE '^(ssh|ecdsa|sk-)' "${file}" || echo 0)))
done

if [ "${keys}" -eq 0 ]; then
  echo "No authorised key found anywhere on this machine."
  echo "Turning passwords off now would lock everybody out. Refusing."
  exit 1
fi
echo "  ${keys} authorised key(s) present"

install -d -m 0755 /etc/ssh/sshd_config.d
cat > "${DROPIN}" <<'CONF'
# Written by node/harden-ssh.sh.
#
# A drop-in rather than an edit of sshd_config: a package upgrade replaces the
# main file and would silently take the change with it.
PasswordAuthentication no
KbdInteractiveAuthentication no

# prohibit-password rather than no: the deploy key and every workflow in this
# project log in as root, and moving that to another account is a change of its
# own. What this stops is the password.
PermitRootLogin prohibit-password
CONF

if ! sshd -t; then
  rm -f "${DROPIN}"
  echo "sshd refuses the configuration. Put back what was there and changed nothing."
  exit 1
fi

systemctl reload ssh 2>/dev/null || systemctl reload sshd

# Read back from the running daemon rather than from the file. A drop-in that
# is present and not in effect looks exactly like one that is.
effective=$(sshd -T | grep -iE '^(passwordauthentication|permitrootlogin|kbdinteractiveauthentication)' | tr '\n' ' ')
echo "  now: ${effective}"

case "${effective}" in
  *"passwordauthentication no"*) echo "  passwords are no longer accepted" ;;
  *) echo "  passwords are still accepted; look at this now"; exit 1 ;;
esac
