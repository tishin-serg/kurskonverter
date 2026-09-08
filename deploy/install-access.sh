#!/usr/bin/env bash
# Administrative setup: bash deploy/install-access.sh /path/to/dedicated-key.pub
set -euo pipefail
[[ "$EUID" == 0 && "$#" == 1 ]] || { echo 'Run as root with a public key file'; exit 1; }
source_dir=$(cd "$(dirname "$0")" && pwd)
key=$(cat "$1")
key=${key%$'\r'}
[[ "$key" =~ ^ssh-ed25519\ [A-Za-z0-9+/=]+(\ [[:print:]]*)?$ && "$key" != *$'\n'* ]] || { echo 'Expected one Ed25519 public key'; exit 1; }
if id kursdeploy >/dev/null 2>&1; then
  [[ "$(id -u kursdeploy)" != 0 && "$(id -Gn kursdeploy)" == kursdeploy ]] || { echo 'Unexpected existing account privileges'; exit 1; }
else
  useradd --system --user-group --home-dir /var/lib/kursdeploy --shell /bin/bash kursdeploy
fi
usermod --lock kursdeploy
usermod --home /var/lib/kursdeploy --shell /bin/bash kursdeploy
install -d -o root -g root -m 755 /var/lib/kursdeploy /var/lib/kursdeploy/.ssh /usr/local/libexec/kurskonverter
install -o root -g root -m 755 "$source_dir/ssh-command.sh" /usr/local/libexec/kurskonverter/ssh-command
install -o root -g root -m 755 "$source_dir/restricted-deploy.sh" /usr/local/sbin/kurskonverter-deploy
install -o root -g root -m 755 "$source_dir/deploy.sh" /usr/local/libexec/kurskonverter/deploy.sh
printf 'restrict,command="/usr/local/libexec/kurskonverter/ssh-command" %s\n' "$key" > /var/lib/kursdeploy/.ssh/authorized_keys
chown root:root /var/lib/kursdeploy/.ssh/authorized_keys
chmod 644 /var/lib/kursdeploy/.ssh/authorized_keys
sudoers=$(mktemp)
trap 'rm -f "$sudoers"' EXIT
printf 'Defaults:kursdeploy env_reset\nkursdeploy ALL=(root) NOPASSWD: /usr/local/sbin/kurskonverter-deploy\n' > "$sudoers"
visudo -cf "$sudoers"
install -o root -g root -m 440 "$sudoers" /etc/sudoers.d/kurskonverter
echo 'Restricted deployment access installed; administrator SSH is unchanged.'
