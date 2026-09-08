#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
digest=$(printf 'a%.0s' {1..64})
for command in '' id bash 'sudo id' 'sftp' \
  "deploy ghcr.io/other/bot@sha256:$digest" \
  'deploy ghcr.io/tishin-serg/kurskonverter:latest' \
  "deploy ghcr.io/tishin-serg/kurskonverter@sha256:$digest; id" \
  "deploy ghcr.io/tishin-serg/kurskonverter@sha256:$digest extra" \
  $'deploy ghcr.io/tishin-serg/kurskonverter@sha256:'"$digest"$'\nid'; do
  result=0
  SSH_ORIGINAL_COMMAND="$command" bash "$root/deploy/ssh-command.sh" >/dev/null 2>&1 || result=$?
  [[ "$result" == 126 ]] || { echo 'Forbidden command was not rejected'; exit 1; }
done
for argument in 'anything' 'ghcr.io/other/bot@sha256:'"$digest"; do
  result=0
  bash "$root/deploy/restricted-deploy.sh" "$argument" >/dev/null 2>&1 || result=$?
  [[ "$result" == 126 ]]
done
echo 'deploy access rejection tests: PASS'
