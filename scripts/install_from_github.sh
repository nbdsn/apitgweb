#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="newapi-jdc"
CONFIG_FILE="/etc/${SERVICE_NAME}.conf"
REPO_OWNER="${REPO_OWNER:-nbdsn}"
REPO_NAME="${REPO_NAME:-apitgweb}"
REPO_BRANCH="${REPO_BRANCH:-codex-jdc-backup-tg}"
RELEASE_TAG="${RELEASE_TAG:-jdc-latest}"
TMP_DIR="$(mktemp -d /tmp/newapi-jdc-install.XXXXXX)"
TARGET_OS="linux"
TARGET_ARCH="amd64"
PREBUILT_NAME=""
PREBUILT_URL=""
ARCHIVE_URL="https://codeload.github.com/${REPO_OWNER}/${REPO_NAME}/tar.gz/refs/heads/${REPO_BRANCH}"
ARCHIVE_PATH="${TMP_DIR}/repo.tar.gz"

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)
      echo amd64
      ;;
    aarch64|arm64)
      echo arm64
      ;;
    *)
      echo amd64
      ;;
  esac
}

download_archive() {
  local url="$1"
  local output="$2"
  curl -fsSL --retry 3 --retry-delay 2 "$url" -o "$output"
}

installed_manager_supports_menu() {
  local manager_path="$1"
  [[ -f "${manager_path}" ]] || return 1
  grep -q "show_manage_menu()" "${manager_path}" || grep -q "进入交互式管理菜单" "${manager_path}"
}

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
    if installed_manager_supports_menu "${APP_DIR}/scripts/jdc_manager.sh"; then
      bash "${APP_DIR}/scripts/jdc_manager.sh" "$@"
      exit $?
    else
      echo "检测到旧版管理脚本，切换到最新引导器..."
    fi
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

TARGET_ARCH="$(detect_arch)"
PREBUILT_NAME="newapi-jdc-${TARGET_OS}-${TARGET_ARCH}.tar.gz"
PREBUILT_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${RELEASE_TAG}/${PREBUILT_NAME}"

if download_archive "${PREBUILT_URL}" "${ARCHIVE_PATH}"; then
  echo "已下载预编译安装包: ${PREBUILT_URL}"
else
  echo "未找到预编译安装包，回退到源码安装包: ${ARCHIVE_URL}"
  download_archive "${ARCHIVE_URL}" "${ARCHIVE_PATH}"
fi

tar -xzf "${ARCHIVE_PATH}" -C "${TMP_DIR}"
SRC_DIR="$(find "${TMP_DIR}" -maxdepth 1 -mindepth 1 -type d | head -n 1)"
if [[ -z "${SRC_DIR}" || ! -f "${SRC_DIR}/scripts/jdc_manager.sh" ]]; then
  echo "源码包结构不正确，未找到 scripts/jdc_manager.sh"
  exit 1
fi

chmod +x "${SRC_DIR}/scripts/jdc_manager.sh"
bash "${SRC_DIR}/scripts/jdc_manager.sh" "$@"
