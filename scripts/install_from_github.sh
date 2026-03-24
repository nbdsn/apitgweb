#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="newapi-jdc"
CONFIG_FILE="/etc/${SERVICE_NAME}.conf"
REPO_OWNER="${REPO_OWNER:-nbdsn}"
REPO_NAME="${REPO_NAME:-apitgweb}"
REPO_BRANCH="${REPO_BRANCH:-codex-jdc-backup-tg}"
TMP_DIR="$(mktemp -d /tmp/newapi-jdc-install.XXXXXX)"
ARCHIVE_URL="https://codeload.github.com/${REPO_OWNER}/${REPO_NAME}/tar.gz/refs/heads/${REPO_BRANCH}"
ARCHIVE_PATH="${TMP_DIR}/repo.tar.gz"

cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

if [[ ${EUID} -ne 0 ]]; then
  echo "请使用 root 运行，或在命令前加 sudo"
  exit 1
fi

if [[ -f "${CONFIG_FILE}" ]]; then
  # shellcheck disable=SC1090
  source "${CONFIG_FILE}"
  if [[ -n "${APP_DIR:-}" && -x "${APP_DIR}/scripts/jdc_manager.sh" ]]; then
    bash "${APP_DIR}/scripts/jdc_manager.sh" "$@"
    exit $?
  fi
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "未检测到 curl，请先安装 curl"
  exit 1
fi

if ! command -v tar >/dev/null 2>&1; then
  echo "未检测到 tar，请先安装 tar"
  exit 1
fi

echo "下载源码中: ${ARCHIVE_URL}"
curl -fsSL "${ARCHIVE_URL}" -o "${ARCHIVE_PATH}"

tar -xzf "${ARCHIVE_PATH}" -C "${TMP_DIR}"
SRC_DIR="$(find "${TMP_DIR}" -maxdepth 1 -mindepth 1 -type d | head -n 1)"
if [[ -z "${SRC_DIR}" || ! -f "${SRC_DIR}/scripts/jdc_manager.sh" ]]; then
  echo "源码包结构不正确，未找到 scripts/jdc_manager.sh"
  exit 1
fi

chmod +x "${SRC_DIR}/scripts/jdc_manager.sh"
bash "${SRC_DIR}/scripts/jdc_manager.sh" "$@"
