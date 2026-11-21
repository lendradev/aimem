#!/usr/bin/env bash
# Build aimem into dist/aimem and optionally install it.
#
# Usage:
#   ./scripts/build.sh            build only
#   ./scripts/build.sh --install  build, then install into the install dir
#
# The install dir defaults to ~/.local/bin on Linux, ~/bin on macOS, and
# %LOCALAPPDATA%\Programs\aimem on Windows (under Git Bash or WSL).
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="$PROJECT_ROOT/dist"

case "$(uname -s)" in
  Darwin) DEFAULT_INSTALL_DIR="$HOME/bin" ;;
  MINGW*|MSYS*|CYGWIN*) DEFAULT_INSTALL_DIR="${LOCALAPPDATA:-$HOME/AppData/Local}/Programs/aimem" ;;
  *) DEFAULT_INSTALL_DIR="$HOME/.local/bin" ;;
esac
INSTALL_DIR="${AIMEM_INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"

mkdir -p "$DIST_DIR"

# The output path must be relative to the sandbox workdir. scripts/go.sh
# mounts the project root at /w inside the container, and an absolute host
# path does not exist there.
"$PROJECT_ROOT/scripts/go.sh" sh -c "go build -trimpath -ldflags '-s -w' -o dist/aimem ./cmd/aimem"

echo "built $DIST_DIR/aimem ($(du -h "$DIST_DIR/aimem" | cut -f1))"

if [ "${1:-}" = "--install" ]; then
  mkdir -p "$INSTALL_DIR"

  # Install to a temp file in the target directory, then rename over the
  # old binary. A rename inside one filesystem is atomic, so the previous
  # binary stays intact and runnable until the new one is fully in place.
  # Writing straight to the target would truncate it first, leaving a window
  # where a running agent could exec a half-written file.
  TMP_BIN="$INSTALL_DIR/.aimem.install.$$"
  trap 'rm -f "$TMP_BIN"' EXIT
  install -m 0755 "$DIST_DIR/aimem" "$TMP_BIN"
  mv -f "$TMP_BIN" "$INSTALL_DIR/aimem"
  trap - EXIT

  echo "installed $INSTALL_DIR/aimem"
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) echo "note: add $INSTALL_DIR to your PATH" ;;
  esac
fi
