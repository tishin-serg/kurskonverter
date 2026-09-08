#!/usr/bin/env bash
# Installed by an administrator; never accept scripts over SSH stdin.
set -Eeuo pipefail
cd "${DEPLOY_DIR:-/opt/kurskonverter}"
exec 9>.deploy.lock
flock -n 9 || { echo 'Another deployment is running'; exit 1; }
image=${1:?immutable image required}
[[ "$image" =~ ^ghcr\.io/[a-z0-9._/-]+@sha256:[a-f0-9]{64}$ ]] || { echo 'Invalid image digest'; exit 1; }
export BOT_IMAGE="$image"
previous=''
if [[ -s current-image ]]; then previous=$(cat current-image); fi
if [[ -n "$previous" ]]; then
  [[ "$previous" =~ ^ghcr\.io/[a-z0-9._/-]+@sha256:[a-f0-9]{64}$ ]] || exit 1
  printf '%s\n' "$previous" > previous-image
fi
wait_ready() {
  local cid status
  for ((i=0;i<24;i++)); do
    cid=$(docker compose ps -q bot)
    if [[ -n "$cid" ]]; then
      status=$(docker inspect --format '{{.State.Health.Status}}' "$cid" 2>/dev/null || true)
      if [[ "$status" == healthy ]] && docker compose exec -T bot bot healthcheck ready; then return 0; fi
    fi
    sleep 5
  done
  return 1
}
rollback() {
  trap - ERR
  echo 'Deployment failed; attempting rollback'
  if [[ -n "$previous" ]]; then
    export BOT_IMAGE="$previous"
    if docker compose up -d --remove-orphans && wait_ready; then
      printf '%s\n' "$previous" > current-image
      echo 'Rollback healthy'
    else
      echo 'ROLLBACK FAILED: operator action required'
    fi
  else
    docker compose stop bot || true
    echo 'No previous image; first deployment stopped'
  fi
  exit 1
}
# Pull before replacing the service; failure leaves the current service intact.
docker pull "$image"
trap rollback ERR
docker compose up -d --remove-orphans
wait_ready
printf '%s\n' "$image" > current-image.tmp
mv current-image.tmp current-image
trap - ERR
echo 'Deployment healthy and ready'


commit=$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$image")
[[ "$commit" =~ ^[a-f0-9]{40}$ ]] && printf 'commit=%s\n' "$commit"

