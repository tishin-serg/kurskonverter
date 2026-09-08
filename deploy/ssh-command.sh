#!/bin/bash -p
# Root-owned forced command. Never evaluate SSH_ORIGINAL_COMMAND as shell code.
set -euo pipefail
if [[ "${SSH_ORIGINAL_COMMAND:-}" =~ ^deploy\ (ghcr\.io/tishin-serg/kurskonverter@sha256:[a-f0-9]{64})$ ]]; then
  exec /usr/bin/sudo -n -- /usr/local/sbin/kurskonverter-deploy "${BASH_REMATCH[1]}"
fi
echo 'Only deploy of a kurskonverter image digest is permitted.' >&2
exit 126
