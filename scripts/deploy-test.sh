#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
temp=$(mktemp -d)
trap 'rm -rf "$temp"' EXIT
mkdir -p "$temp/bin"
export MOCK_STATE="$temp/state"
export OLD="ghcr.io/test/bot@sha256:$(printf 'a%.0s' {1..64})"
export NEW="ghcr.io/test/bot@sha256:$(printf 'b%.0s' {1..64})"
cat > "$temp/bin/docker" <<'MOCK'
#!/usr/bin/env bash
set -eu
case "$*" in
  'pull '*) [[ "${SCENARIO}" != pullfail ]];;
  'compose up '*) printf '%s' "$BOT_IMAGE" > "$MOCK_STATE";;
  'compose ps -q bot') echo container;;
  'image inspect '*) printf 'c%.0s' {1..40}; echo;;
  'inspect '*) echo healthy;;
  'compose exec -T bot bot healthcheck ready') [[ "$SCENARIO" != rollbackfail ]] && { [[ "$SCENARIO" != fail && "$SCENARIO" != firstfail ]] || [[ "$(cat "$MOCK_STATE")" == "$OLD" ]]; };;
  'compose stop bot') echo stopped > "$MOCK_STATE";;
  *) echo "Unexpected mock command: $*" >&2; exit 1;;
esac
MOCK
printf '#!/usr/bin/env bash\nexit 0\n' > "$temp/bin/sleep"
# Git Bash may not ship flock; only lock plumbing is stubbed, production uses flock.
printf '#!/usr/bin/env bash\nexit 0\n' > "$temp/bin/flock"
chmod +x "$temp/bin/"*
export PATH="$temp/bin:$PATH"
for SCENARIO in success fail firstfail pullfail rollbackfail; do
  export SCENARIO
  export DEPLOY_DIR="$temp/$SCENARIO"
  mkdir -p "$DEPLOY_DIR"
  printf '%s' "$OLD" > "$MOCK_STATE"
  if [[ "$SCENARIO" != firstfail ]]; then printf '%s\n' "$OLD" > "$DEPLOY_DIR/current-image"; fi
  result=0
  bash "$root/deploy/deploy.sh" "$NEW" > "$temp/output" 2>&1 || result=$?
  case "$SCENARIO" in
    success) [[ $result == 0 && "$(cat "$DEPLOY_DIR/current-image")" == "$NEW" ]];;
    fail|pullfail) [[ $result != 0 && "$(cat "$MOCK_STATE")" == "$OLD" && "$(cat "$DEPLOY_DIR/current-image")" == "$OLD" ]];;
    firstfail) [[ $result != 0 && "$(cat "$MOCK_STATE")" == stopped ]];;
    rollbackfail) [[ $result != 0 ]]; grep -q 'ROLLBACK FAILED' "$temp/output";;
  esac
  echo "deploy $SCENARIO: PASS"
done

bash "$root/scripts/deploy-access-test.sh"
