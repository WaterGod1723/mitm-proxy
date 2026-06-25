#!/bin/bash

# 构建各平台二进制文件脚本
# 使用方法: ./build.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

OUTPUT_DIR="${SCRIPT_DIR}/dist"
mkdir -p "$OUTPUT_DIR"

APP_NAME="mitm-proxy-rewrite-v2"

VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "v$(date +%Y%m%d)")
COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')

LDFLAGS="-s -w -X main.Version=${VERSION} -X main.CommitHash=${COMMIT_HASH} -X main.BuildTime=${BUILD_TIME}"

PLATFORMS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
)

echo "========================================="
echo "开始构建 ${APP_NAME}"
echo "版本: ${VERSION}"
echo "Commit: ${COMMIT_HASH}"
echo "构建时间: ${BUILD_TIME}"
echo "输出目录: ${OUTPUT_DIR}"
echo "========================================="

for platform in "${PLATFORMS[@]}"; do
    GOOS="${platform%/*}"
    GOARCH="${platform#*/}"

    if [ "$GOOS" = "windows" ]; then
        OUTPUT_NAME="${APP_NAME}_${GOOS}_${GOARCH}.exe"
    else
        OUTPUT_NAME="${APP_NAME}_${GOOS}_${GOARCH}"
    fi

    OUTPUT_PATH="${OUTPUT_DIR}/${OUTPUT_NAME}"

    echo ""
    echo "正在构建: ${GOOS}/${GOARCH} -> ${OUTPUT_NAME}"

    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
        go build -ldflags "$LDFLAGS" -o "$OUTPUT_PATH" main.go

    if [ "$GOOS" != "windows" ]; then
        chmod +x "$OUTPUT_PATH"
    fi

    echo "✓ 构建完成: ${OUTPUT_PATH}"
done

echo ""
echo "========================================="
echo "所有平台构建完成！"
echo "========================================="
echo ""
echo "生成的文件："
ls -lh "$OUTPUT_DIR"
