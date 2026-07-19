#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/remote-latexmk-paper-smoke.XXXXXX")"
ACTIVE_PROJECT=""

SLIM_SERVER_IMAGE="${SMOKE_SLIM_SERVER_IMAGE:-ghcr.io/inviscat/remote-latexmk-server@sha256:bb55b20104a85825742ea42b7deb4d1ed3598afe9462736ffb6634ce0b95e1a2}"
FULL_SERVER_IMAGE="${SMOKE_FULL_SERVER_IMAGE:-ghcr.io/inviscat/remote-latexmk-server-full@sha256:1a114db5b2974a16c17faf7171b32ac2c00421b19d1d35e8dcbaa090f1e2b536}"
CLIENT_IMAGE="${SMOKE_CLIENT_IMAGE:-ghcr.io/inviscat/remote-latexmk-client@sha256:8e64efbfe8020f64f5e5f04876a5825e9b6539f79eaadc784c9cd3f507cc047f}"

dc() {
  docker compose \
    --env-file /dev/null \
    --project-directory "$ROOT" \
    --project-name "$ACTIVE_PROJECT" \
    -f "$ROOT/compose.yaml" \
    -f "$ROOT/compose.ghcr.yaml" \
    "$@"
}

stop_stack() {
  if [[ -n "$ACTIVE_PROJECT" ]]; then
    if dc --profile client --profile watch --profile https \
      down --volumes --remove-orphans; then
      ACTIVE_PROJECT=""
      return 0
    fi
    echo "paper smoke: failed to remove Compose project $ACTIVE_PROJECT" >&2
    return 1
  fi
}

cleanup() {
  local status=$?
  trap - EXIT
  if ! stop_stack; then
    if [[ "$status" -eq 0 ]]; then
      status=1
    fi
  fi
  if [[ "${KEEP_SMOKE_ARTIFACTS:-0}" == "1" ]]; then
    echo "smoke artifacts kept at: $TMP" >&2
  else
    if ! rm -rf "$TMP"; then
      echo "paper smoke: failed to remove temporary directory $TMP" >&2
      if [[ "$status" -eq 0 ]]; then
        status=1
      fi
    fi
  fi
  exit "$status"
}
trap cleanup EXIT

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "paper smoke: missing required command: $1" >&2
    exit 2
  }
}

assert_contains() {
  local file="$1"
  local text="$2"
  grep -F -- "$text" "$file" >/dev/null || {
    echo "paper smoke: expected '$text' in $file" >&2
    sed -n '1,160p' "$file" >&2
    exit 1
  }
}

wait_for_url() {
  local url="$1"
  local output_file="$2"
  local attempt
  for ((attempt = 1; attempt <= 60; attempt++)); do
    if curl --fail --silent "$url" >"$output_file" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  echo "paper smoke: service did not become ready: $url" >&2
  curl --fail --show-error "$url" >"$output_file"
}

assert_manifest() {
  local file="$1"
  local count="$2"
  shift 2
  assert_contains "$file" "resolved: true"
  assert_contains "$file" "files: $count"
  local path
  for path in "$@"; do
    assert_contains "$file" "  $path  ("
  done
  if grep -F -- 'remote-compilation.svg' "$file" >/dev/null; then
    echo "paper smoke: editable SVG unexpectedly entered the upload manifest" >&2
    exit 1
  fi
}

run_case() {
  local name="$1"
  local profile="$2"
  local engines="$3"
  local server_image="$4"
  local expected_count="$5"
  shift 5
  local expected=("$@")
  local workspace="$TMP/$name/workspace"
  local records="$TMP/$name/records"
  local output="$workspace/.smoke-output"
  local download="$workspace/.smoke-download"
  local port="${SMOKE_PORT:-18080}"

  mkdir -p "$workspace" "$records" "$output" "$download"
  cp -R "$ROOT/examples/$name/." "$workspace/"
  if grep -E -i 'smoke test|test fixture|not a real academic paper' \
    "$workspace/main.tex" \
    "$workspace/sections/introduction.tex" \
    "$workspace/sections/method.tex" >/dev/null; then
    echo "paper smoke: [$name] test disclosure must stay in the final acknowledgment" >&2
    exit 1
  fi

  ACTIVE_PROJECT="remote-latexmk-${name}-smoke-$$-${RANDOM}"
  export LATEXMK_API_TOKEN="remote-latexmk-${name}-smoke-token-0000000000000000"
  export LATEXMK_BIND_ADDRESS="127.0.0.1"
  export LATEXMK_HOST_PORT="$port"
  export LATEXMK_PROJECT_DIR="$workspace"
  export LATEXMK_CLIENT_UID="$(id -u)"
  export LATEXMK_CLIENT_GID="$(id -g)"
  export LATEXMK_CLIENT_SERVER="http://server:8080"
  export LATEXMK_CLIENT_TOKEN="$LATEXMK_API_TOKEN"
  export LATEXMK_CLIENT_CA_FILE=""
  export LATEXMK_CLIENT_UPLOAD_MODE="auto"
  export LATEXMK_CLIENT_MANIFEST_FILE=""
  export LATEXMK_GHCR_SERVER_IMAGE="$server_image"
  export LATEXMK_GHCR_CLIENT_IMAGE="$CLIENT_IMAGE"
  export LATEXMK_SERVER_PLATFORM="linux/amd64"
  export LATEXMK_IMAGE_PROFILE="$profile"
  export LATEXMK_ENGINES="$engines"
  export LATEXMK_COMPILE_TIMEOUT="10m"
  export CADDY_IMAGE="caddy:2-alpine@sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648"

  echo "paper smoke: [$name] pull pinned images"
  dc pull --quiet server client gateway

  echo "paper smoke: [$name] dependency discovery without a server"
  dc run --rm --no-deps client files --engine pdflatex main.tex >"$records/manifest-before.txt"
  assert_manifest "$records/manifest-before.txt" "$expected_count" "${expected[@]}"
  dc run --rm --no-deps client compile --engine pdflatex --dry-run main.tex >"$records/dry-run.txt"
  cmp "$records/manifest-before.txt" "$records/dry-run.txt"

  echo "paper smoke: [$name] start isolated Compose stack"
  dc up -d --no-build --quiet-pull server gateway
  wait_for_url "http://127.0.0.1:$port/healthz" "$records/health.json"
  wait_for_url "http://127.0.0.1:$port/readyz" "$records/ready.json"

  dc run --rm --no-deps client meta --json >"$records/meta.json"
  assert_contains "$records/meta.json" '"service":"remote-latexmk"'
  assert_contains "$records/meta.json" "\"imageProfile\":\"$profile\""
  assert_contains "$records/meta.json" '"pdflatex"'
  assert_contains "$records/meta.json" '"shellEscapeAllowed":false'

  echo "paper smoke: [$name] compile, wait, and download artifacts"
  dc run --rm --no-deps client compile \
    --engine pdflatex \
    --timeout 10m \
    --out-dir /workspace/.smoke-output \
    --json \
    main.tex >"$records/result.json"
  assert_contains "$records/result.json" '"success":true'
  assert_contains "$records/result.json" '"engine":"pdflatex"'
  assert_contains "$records/result.json" '"path":"main.pdf"'
  assert_contains "$records/result.json" '"path":"main.bbl"'

  local job_id
  job_id="$(sed -n 's/.*"requestId":"\([^"]*\)".*/\1/p' "$records/result.json")"
  if [[ -z "$job_id" ]]; then
    echo "paper smoke: could not read the job ID" >&2
    exit 1
  fi

  test -s "$output/main.pdf"
  test -s "$output/main.bbl"
  test -s "$output/main.log"
  test "$(dd if="$output/main.pdf" bs=5 count=1 2>/dev/null)" = '%PDF-'
  grep -F '\bibitem' "$output/main.bbl" >/dev/null
  if grep -E -i 'undefined (citation|citations|reference|references)|citation.+undefined' "$output/main.log" >/dev/null; then
    echo "paper smoke: unresolved citation or reference in $name" >&2
    exit 1
  fi

  echo "paper smoke: [$name] result, log, diagnostic, and artifact APIs"
  dc run --rm --no-deps client jobs show "$job_id" --json >"$records/job.json"
  dc run --rm --no-deps client logs "$job_id" --source all --tail 300 --max-bytes 131072 --json >"$records/logs.json"
  dc run --rm --no-deps client diagnostics "$job_id" --json >"$records/diagnostics.json"
  dc run --rm --no-deps client artifacts list "$job_id" >"$records/artifacts.txt"
  assert_contains "$records/job.json" '"ok":true'
  assert_contains "$records/job.json" '"status":"succeeded"'
  assert_contains "$records/logs.json" '"ok":true'
  assert_contains "$records/diagnostics.json" '"ok":true'
  assert_contains "$records/artifacts.txt" $'application/pdf\tmain.pdf'

  local pdf_id
  pdf_id="$(awk '$4 == "main.pdf" { print $1; exit }' "$records/artifacts.txt")"
  if [[ ! "$pdf_id" =~ ^[0-9a-f]{32}$ ]]; then
    echo "paper smoke: invalid PDF artifact ID: $pdf_id" >&2
    exit 1
  fi
  dc run --rm --no-deps client artifacts get "$job_id" "$pdf_id" \
    --out-dir /workspace/.smoke-download >"$records/download.txt"
  cmp "$output/main.pdf" "$download/main.pdf"

  dc run --rm --no-deps client files --engine pdflatex main.tex >"$records/manifest-after.txt"
  assert_manifest "$records/manifest-after.txt" "$expected_count" "${expected[@]}"

  if command -v pdfinfo >/dev/null 2>&1; then
    pdfinfo "$output/main.pdf" >"$records/pdfinfo.txt"
  fi
  if command -v pdftotext >/dev/null 2>&1; then
    pdftotext "$output/main.pdf" "$records/paper.txt"
    tr '\n' ' ' <"$records/paper.txt" >"$records/paper-one-line.txt"
    assert_contains "$records/paper-one-line.txt" 'SMOKE TEST'
    assert_contains "$records/paper-one-line.txt" 'FIXTURE, not a real academic paper'
  fi
  if command -v pdftoppm >/dev/null 2>&1; then
    pdftoppm -f 1 -singlefile -png -r 150 \
      "$output/main.pdf" "$records/page-1" >/dev/null
  fi

  stop_stack
  echo "paper smoke: [$name] passed"
}

require docker
require curl
docker info >/dev/null
docker compose version >/dev/null

run_case \
  slim \
  xelatex-cjk-slim \
  xelatex,pdflatex \
  "$SLIM_SERVER_IMAGE" \
  6 \
  figures/remote-compilation.png \
  main.tex \
  references.bib \
  sections/introduction.tex \
  sections/method.tex \
  sections/results.tex

run_case \
  ieee \
  texlive-full \
  xelatex,lualatex,pdflatex \
  "$FULL_SERVER_IMAGE" \
  7 \
  code/procedure.txt \
  figures/remote-compilation.png \
  main.tex \
  references.bib \
  sections/introduction.tex \
  sections/method.tex \
  sections/results.tex

echo "paper smoke: all executable examples passed"
