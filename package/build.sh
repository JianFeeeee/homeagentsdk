#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${PROJECT_ROOT}/build"
VERSION="${VERSION:-$(git -C "$PROJECT_ROOT" describe --tags --dirty 2>/dev/null || echo "0.7.1")}"
GO="${GO:-$(command -v go 2>/dev/null || echo "/home/jianf/go1.26.5/go/bin/go")}"
GOCACHE="${GOCACHE:-}"
GOPATH="${GOPATH:-}"

TARGET="${1:-native}"
COMPONENT="${2:-all}"

case "$TARGET" in
  native)   GOOS="" GOARCH="" ;;
  linux/amd64)  GOOS=linux   GOARCH=amd64 ;;
  linux/arm64)  GOOS=linux   GOARCH=arm64 ;;
  darwin/amd64) GOOS=darwin  GOARCH=amd64 ;;
  darwin/arm64) GOOS=darwin  GOARCH=arm64 ;;
  windows/amd64) GOOS=windows GOARCH=amd64 ;;
  all)
    "$0" linux/amd64   "$COMPONENT"
    "$0" linux/arm64   "$COMPONENT"
    "$0" darwin/amd64  "$COMPONENT"
    "$0" darwin/arm64  "$COMPONENT"
    "$0" windows/amd64 "$COMPONENT"
    exit 0
    ;;
  *)
    echo "Unknown target: $TARGET"
    echo "Usage: $0 [native|linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64|all] [all|plugindev]"
    exit 1
esac

if [ -n "${GOOS:-}" ]; then
  SUFFIX="${GOOS}_${GOARCH}"
  export GOOS GOARCH
fi
export CGO_ENABLED=0
[ -n "$GOCACHE" ] && export GOCACHE
[ -n "$GOPATH" ] && export GOPATH

mkdir -p "$BUILD_DIR"

build_plugindev() {
  local src="tools/plugindev"
  local out="$BUILD_DIR/plugindev${SUFFIX:+_$SUFFIX}"
  if [ "$GOOS" = "windows" ]; then out="${out}.exe"; fi

  echo "[BUILD] plugindev ${GOOS:-linux}/${GOARCH:-amd64} → $out"
  cd "$PROJECT_ROOT/$src"
  "$GO" build -trimpath -ldflags "-X gitcode.com/JianFeeeee/homeagent-sdk/meta.Version=${VERSION}" \
    -o "$out" .
  echo "  OK ($(du -h "$out" | cut -f1))"
  cd "$PROJECT_ROOT"
}

case "$COMPONENT" in
  all|plugindev) build_plugindev ;;
  *)
    echo "Unknown component: $COMPONENT"
    exit 1
esac
