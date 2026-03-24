#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT}/dist"
STAMP="$(date +%Y%m%d-%H%M%S)"
TMP_BASE="${TMPDIR:-/tmp}"
PKG_DIR="${TMP_BASE}/newapi-jdc-src-${STAMP}"
ARCHIVE="${OUT_DIR}/newapi-jdc-src-${STAMP}.tar.gz"

rm -rf "${PKG_DIR}"
mkdir -p "${PKG_DIR}"
mkdir -p "${OUT_DIR}"

cp -R "${ROOT}/." "${PKG_DIR}/"
rm -rf \
  "${PKG_DIR}/.git" \
  "${PKG_DIR}/dist" \
  "${PKG_DIR}/web/node_modules" \
  "${PKG_DIR}/node_modules" \
  "${PKG_DIR}/未命名文件夹"

mkdir -p "${PKG_DIR}/dist"
tar -czf "${ARCHIVE}" -C "$(dirname "${PKG_DIR}")" "$(basename "${PKG_DIR}")"
rm -rf "${PKG_DIR}"
echo "已生成源码安装包: ${ARCHIVE}"
