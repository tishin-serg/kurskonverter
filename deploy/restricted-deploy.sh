#!/bin/bash -p
set -euo pipefail
[[ "$EUID" == 0 && "$#" == 1 ]] || exit 126
[[ "$1" =~ ^ghcr\.io/tishin-serg/kurskonverter@sha256:[a-f0-9]{64}$ ]] || exit 126
# Discard caller-controlled Docker, Compose, Bash, PATH and deployment settings.
exec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin HOME=/root \
  /bin/bash /usr/local/libexec/kurskonverter/deploy.sh "$1"
