#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$DIR"

mkdir -p dist

TARGETS=(
    "linux/amd64"
    "linux/arm64"
    "linux/386"
    "linux/arm"
)

echo "=== 开始编译 AimiliVPN 多架构发行二进制文件 ==="

for TARGET in "${TARGETS[@]}"; do
    OS="${TARGET%/*}"
    ARCH="${TARGET#*/}"
    OUTPUT="dist/aimilivpn_${OS}_${ARCH}"
    echo "-> 正在编译 ${TARGET} ..."
    CGO_ENABLED=0 GOOS="$OS" GOARCH="$ARCH" go build -ldflags="-s -w" -o "$OUTPUT" ./cmd/aimilivpn
    gzip -kf "$OUTPUT"
done

echo "=== 编译完成，产物位于 dist/ 目录 ==="
ls -lh dist/
