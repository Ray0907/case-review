#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
port=18097
if lsof -nP -iTCP:$port -sTCP:LISTEN -t >/dev/null 2>&1; then echo "port $port busy" >&2; exit 1; fi
scratch=$(mktemp -d)
pid=
cleanup() {
  if [[ -n "$pid" ]]; then kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
  trash "$scratch"
}
trap cleanup EXIT
(cd "$root/frontend" && npm run build)
(cd "$root/backend" && go build -o "$scratch/harbor" ./cmd/server)
PORT=$port DATA_DIR="$scratch/data" PIPELINE_MODE=fake STATIC_DIR="$root/frontend/dist" "$scratch/harbor" > "$scratch/server.log" 2>&1 &
pid=$!
ready=false
for _ in $(seq 100); do
  if curl -fsS -o /dev/null "http://127.0.0.1:$port/api/health" 2>/dev/null; then ready=true; break; fi
  if ! kill -0 "$pid" 2>/dev/null; then break; fi
  sleep 0.1
done
if [[ "$ready" != true ]]; then echo "server did not start; check port and fake-mode configuration" >&2; exit 1; fi
artifacts="$root/artifacts/e2e/$(cd "$root" && git describe --tags --always)"
mkdir -p "$artifacts"
for file in "$artifacts"/step-*.png "$artifacts"/summary.txt "$artifacts"/results.json "$artifacts"/server-debug.log; do
  if [[ -f "$file" ]]; then trash "$file"; fi
done
mkdir -p "$scratch/fixtures"
cp "$root/frontend/e2e/fixtures/"* "$scratch/fixtures/"
node -e 'require("fs").writeFileSync(process.argv[1], JSON.stringify({base:process.argv[2],fixtures:process.argv[3],out:process.argv[4],documents:process.argv[5]}))' \
  "$scratch/e2e-env.json" "http://127.0.0.1:$port" "$scratch/fixtures" "$artifacts" "$root/testdata/documents"
{ printf 'const envFile = %s;\n' "$(node -p 'JSON.stringify(process.argv[1])' "$scratch/e2e-env.json")"; cat "$root/frontend/e2e/review.ego.mjs"; } > "$scratch/review.ego.mjs"
if ! ego-browser nodejs < "$scratch/review.ego.mjs"; then
  cp "$scratch/server.log" "$artifacts/server-debug.log"
  exit 1
fi
