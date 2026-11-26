#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
MODULE_DIR="$REPO_ROOT/experiments/gvisor"
OUTPUT_DIR="${1:-$REPO_ROOT/build/macos}"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

mkdir -p "$OUTPUT_DIR"
HEADER_PATH="$OUTPUT_DIR/libgo_packet_engine.h"

for ARCH in arm64 amd64; do
  ARCH_DIR="$TEMP_DIR/$ARCH"
  mkdir -p "$ARCH_DIR"
  echo "Building Go packet engine for darwin/$ARCH"
  (
    cd "$MODULE_DIR"
    CGO_ENABLED=1 GOOS=darwin GOARCH="$ARCH" \
      CC="clang -arch $([ "$ARCH" = amd64 ] && echo x86_64 || echo arm64)" \
      go build -trimpath -buildmode=c-shared \
        -o "$ARCH_DIR/libgo_packet_engine.dylib" ./cshared
  )
  if [[ "$ARCH" == arm64 ]]; then
    cp "$ARCH_DIR/libgo_packet_engine.h" "$HEADER_PATH"
  else
    cmp -s "$HEADER_PATH" "$ARCH_DIR/libgo_packet_engine.h" || {
      echo "Generated C headers differ between architectures" >&2
      exit 1
    }
  fi
done

lipo -create \
  "$TEMP_DIR/arm64/libgo_packet_engine.dylib" \
  "$TEMP_DIR/amd64/libgo_packet_engine.dylib" \
  -output "$OUTPUT_DIR/libgo_packet_engine.dylib"

lipo -verify_arch arm64 x86_64 "$OUTPUT_DIR/libgo_packet_engine.dylib"
nm -gU "$OUTPUT_DIR/libgo_packet_engine.dylib" | grep -E ' _GoEngine(Create|Start|ProcessIPVersion|ReadPacket|Stop|Destroy)$'

echo "Universal macOS library: $OUTPUT_DIR/libgo_packet_engine.dylib"
echo "C header: $HEADER_PATH"
