#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${JDC_OUT_DIR:-${ROOT}/dist}"
TARGET_OS="${JDC_TARGET_OS:-linux}"
TARGET_ARCH="${JDC_TARGET_ARCH:-amd64}"
PACKAGE_VERSION="${JDC_PACKAGE_VERSION:-$(date +%Y%m%d-%H%M%S)}"
ARCHIVE_NAME="${JDC_ARCHIVE_NAME:-newapi-jdc-${TARGET_OS}-${TARGET_ARCH}.tar.gz}"
BUILD_DIR="${ROOT}/.jdc-build/${TARGET_OS}-${TARGET_ARCH}"
PKG_NAME="newapi-jdc-${TARGET_OS}-${TARGET_ARCH}-${PACKAGE_VERSION}"
PKG_DIR="${BUILD_DIR}/${PKG_NAME}"
ARCHIVE_PATH="${OUT_DIR}/${ARCHIVE_NAME}"

mkdir -p "${OUT_DIR}"
rm -rf "${PKG_DIR}"
mkdir -p "${PKG_DIR}/scripts" "${PKG_DIR}/docs"

pushd "${ROOT}/web" >/dev/null
npm install --legacy-peer-deps
DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION="${PACKAGE_VERSION}" npm run build
popd >/dev/null

pushd "${ROOT}" >/dev/null
GOOS="${TARGET_OS}" GOARCH="${TARGET_ARCH}" go build \
  -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=${PACKAGE_VERSION}'" \
  -o "${PKG_DIR}/newapi" .
popd >/dev/null

cp "${ROOT}/scripts/jdc_manager.sh" "${PKG_DIR}/scripts/jdc_manager.sh"
cp "${ROOT}/scripts/install_from_github.sh" "${PKG_DIR}/scripts/install_from_github.sh"
cp "${ROOT}/docs/jdc-install.md" "${PKG_DIR}/docs/jdc-install.md"
cp "${ROOT}/README.md" "${PKG_DIR}/README.md"
cp "${ROOT}/README.zh_CN.md" "${PKG_DIR}/README.zh_CN.md"
cp "${ROOT}/VERSION" "${PKG_DIR}/VERSION"

chmod +x "${PKG_DIR}/newapi" "${PKG_DIR}/scripts/jdc_manager.sh" "${PKG_DIR}/scripts/install_from_github.sh"

rm -f "${ARCHIVE_PATH}"
tar -czf "${ARCHIVE_PATH}" -C "${BUILD_DIR}" "${PKG_NAME}"

cat > "${OUT_DIR}/jdc-build-info.txt" <<EOF
PACKAGE_VERSION=${PACKAGE_VERSION}
TARGET_OS=${TARGET_OS}
TARGET_ARCH=${TARGET_ARCH}
ARCHIVE_NAME=${ARCHIVE_NAME}
ARCHIVE_PATH=${ARCHIVE_PATH}
EOF

echo "已生成预编译安装包: ${ARCHIVE_PATH}"
