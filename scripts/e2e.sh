#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${PORT:-18080}"
TMP="$(mktemp -d)"
SERVER_PID=""
cleanup() {
  if [[ -n "$SERVER_PID" ]]; then kill "$SERVER_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

go build -o "$TMP/latexmk-server" "$ROOT/packages/server/cmd/server"
go build -o "$TMP/rlatexmk" "$ROOT/packages/cli/cmd/rlatexmk"
cp -R "$ROOT/examples/basic" "$TMP/project"

PORT="$PORT" LATEXMK_AUTH_MODE=none LATEXMK_IMAGE_PROFILE=e2e-local "$TMP/latexmk-server" >"$TMP/server.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 50); do
  if curl -fsS "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1; then break; fi
  sleep 0.1
done

(
  cd "$TMP/project"
  "$TMP/rlatexmk" compile --server "http://127.0.0.1:$PORT" --project-root . main.tex
  test -s main.pdf
  "$TMP/rlatexmk" meta --server "http://127.0.0.1:$PORT" --json >/dev/null
)

echo "e2e passed: $TMP/project/main.pdf"
