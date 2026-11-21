#!/usr/bin/env bash
# Run a Go command with the right toolchain for this machine.
#
# If a Go toolchain is installed, the command runs natively. If not, the script
# falls back to a Docker container so contributors do not have to install Go.
# The module cache and build cache are reused between runs, so dependencies are
# downloaded only once.
#
# Usage:
#   ./scripts/go.sh build ./...
#   ./scripts/go.sh test ./...
#   ./scripts/go.sh vet ./...
#   ./scripts/go.sh run ./cmd/spike
#
# To run a shell snippet in the same sandbox, use the sh form:
#   ./scripts/go.sh sh -c 'go test ./... -count=1'
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_IMAGE="${AIMEM_GO_IMAGE:-golang:1.26.10-alpine}"

if [ "$#" -eq 0 ]; then
  echo "usage: $0 <go-subcommand> [args...]" >&2
  exit 2
fi

# The sh form runs an arbitrary command in the sandbox and is not prefixed
# with "go", because the caller writes the full command itself.
if [ "$1" = "sh" ]; then
  shift
  if command -v go >/dev/null 2>&1; then
    cd "$PROJECT_ROOT"
    exec sh "$@"
  fi
  SANDBOX_CMD=(sh "$@")
else
  SANDBOX_CMD=(go "$@")
fi

if command -v go >/dev/null 2>&1; then
  cd "$PROJECT_ROOT"
  exec "${SANDBOX_CMD[@]}"
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "neither go nor docker found" >&2
  echo "install Go from https://go.dev/dl/ or install Docker" >&2
  exit 1
fi

case "$(uname -s)" in
  Darwin) CACHE_ROOT="${AIMEM_GO_CACHE:-$HOME/Library/Caches/aimem-go}" ;;
  *)      CACHE_ROOT="${AIMEM_GO_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/aimem-go}" ;;
esac

mkdir -p "$CACHE_ROOT/pkg/mod" "$CACHE_ROOT/cache/go-build" "$CACHE_ROOT/tmp"

docker run --rm --name sbx-aimem-go \
  --user "$(id -u):$(id -g)" \
  --cap-drop=ALL --security-opt no-new-privileges --read-only \
  --memory 2g --cpus 2 --pids-limit 256 \
  --tmpfs /tmp:rw,exec,nosuid,nodev,size=512m,mode=1777 \
  --mount "type=bind,source=$CACHE_ROOT/pkg/mod,target=/go/pkg/mod" \
  --mount "type=bind,source=$CACHE_ROOT/cache/go-build,target=/go/.cache/go-build" \
  --mount "type=bind,source=$CACHE_ROOT/tmp,target=/gotmp" \
  -e HOME=/tmp -e GOPATH=/go -e GOMODCACHE=/go/pkg/mod -e GOCACHE=/go/.cache/go-build \
  -e GOTMPDIR=/gotmp -e GOFLAGS=-mod=mod \
  -v "$PROJECT_ROOT":/w -w /w \
  "$GO_IMAGE" "${SANDBOX_CMD[@]}"
