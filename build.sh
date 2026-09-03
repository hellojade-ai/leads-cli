#!/usr/bin/env bash
# Cross-compile the hellojade CLI for every supported platform into ./dist,
# then write a SHA256SUMS file over the results.
#
#   ./build.sh                      # all six targets
#   ./build.sh linux/amd64          # just one
#   VERSION=v0.2.0 ./build.sh       # stamp a version other than the git description
#
# The binaries are static: CGO is off, so there is no libc dependency and the
# same linux/amd64 build runs on Alpine and on Debian.
set -euo pipefail

cd "$(dirname "$0")"

OUT=${OUT:-dist}
BIN=hellojade
PKG=./cmd/hellojade

# Prefer the machine's jade toolchain when it is present; fall back to whatever
# `go` is on PATH (which is what GitHub Actions has).
GO=${GO:-}
if [ -z "$GO" ]; then
  if command -v jo >/dev/null 2>&1; then GO=jo; else GO=go; fi
fi
export GOFLAGS=${GOFLAGS:--mod=readonly}

VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}

TARGETS=("$@")
if [ ${#TARGETS[@]} -eq 0 ]; then
  TARGETS=(linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64)
fi

mkdir -p "$OUT"
echo "building $BIN $VERSION with $($GO version)"

for target in "${TARGETS[@]}"; do
  goos=${target%%/*}
  goarch=${target##*/}
  ext=""
  [ "$goos" = windows ] && ext=".exe"
  name="${BIN}_${VERSION}_${goos}_${goarch}${ext}"

  # -s -w strips the symbol table and DWARF: nothing here reads them, and it
  # is about a third off the binary a partner has to download.
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    "$GO" build -trimpath -ldflags "-s -w" -o "$OUT/$name" "$PKG"

  printf '  %-44s %s\n' "$name" "$(du -h "$OUT/$name" | cut -f1)"
done

( cd "$OUT" && sha256sum "${BIN}"_* > SHA256SUMS )
echo
echo "wrote $OUT/SHA256SUMS"
cat "$OUT/SHA256SUMS"
