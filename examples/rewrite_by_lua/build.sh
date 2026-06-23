#!/bin/bash

# 构建各平台二进制文件脚本
# 使用方法: ./build.sh

set -e

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 输出目录
OUTPUT_DIR="${SCRIPT_DIR}/dist"
mkdir -p "$OUTPUT_DIR"

# 项目名称
APP_NAME="mitm-proxy-rewrite"

# 获取版本号（使用 git tag 或当前日期）
VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "v$(date +%Y%m%d)")

# 获取当前 commit hash
COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# 构建时间
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')

# Go 构建参数
LDFLAGS="-s -w -X main.Version=${VERSION} -X main.CommitHash=${COMMIT_HASH} -X main.BuildTime=${BUILD_TIME}"

# 定义目标平台
# 格式: os/arch
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

# 遍历所有平台进行构建
for platform in "${PLATFORMS[@]}"; do
    GOOS="${platform%/*}"
    GOARCH="${platform#*/}"
    
    # 设置输出文件名
    if [ "$GOOS" = "windows" ]; then
        OUTPUT_NAME="${APP_NAME}_${GOOS}_${GOARCH}.exe"
    else
        OUTPUT_NAME="${APP_NAME}_${GOOS}_${GOARCH}"
    fi
    
    OUTPUT_PATH="${OUTPUT_DIR}/${OUTPUT_NAME}"
    
    echo ""
    echo "正在构建: ${GOOS}/${GOARCH} -> ${OUTPUT_NAME}"
    
    # 执行构建
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
        go build -ldflags "$LDFLAGS" -o "$OUTPUT_PATH" main.go
    
    # 为 Unix 系统添加执行权限
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
