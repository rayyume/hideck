#!/bin/sh
# Build a musl-static HiDeck binary for OpenWrt. Do not UPX: UPX 5 stubs
# need glibc and fail on musl with "Not a valid dynamic program".
#
# Required env: OUT
# Optional: VERSION BUILD_TIME GOARCH GOARM CC
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT"

VERSION=${VERSION:-$(git describe --tags --always 2>/dev/null || echo unknown)}
BUILD_TIME=${BUILD_TIME:-$(date "+%Y-%m-%d %H:%M:%S")}
GOARCH=${GOARCH:-amd64}
GOARM=${GOARM:-}
OUT=${OUT:?OUT is required}
CC=${CC:-musl-gcc}

if [ ! -d internal/web/dist ]; then
  printf 'missing internal/web/dist; build frontend first\n' >&2
  exit 1
fi

mkdir -p "$(dirname -- "$OUT")"

export GOWORK=off
export CGO_ENABLED=1
export GOOS=linux
export GOARCH
if [ -n "${GOARM:-}" ]; then
  export GOARM
else
  unset GOARM
fi
export CC

go build -trimpath -buildvcs=false -tags "with_utls nomsgpack netgo osusergo" \
  -ldflags "-s -w -linkmode external -extldflags -static -X 'github.com/yibaiba/hideck/internal/global.Version=${VERSION}' -X 'github.com/yibaiba/hideck/internal/global.BuildTime=${BUILD_TIME}'" \
  -o "$OUT" ./cmd/hideck

if command -v file >/dev/null 2>&1; then
  file "$OUT"
  if ! file "$OUT" | grep -q 'statically linked'; then
    printf 'OpenWrt binary is not statically linked:\n' >&2
    file "$OUT" >&2
    exit 1
  fi
fi
