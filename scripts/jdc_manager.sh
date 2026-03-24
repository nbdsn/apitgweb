#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="newapi-jdc"
CONFIG_FILE="/etc/${SERVICE_NAME}.conf"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACKAGE_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

random_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 16
  else
    date +%s%N | sha256sum | awk '{print $1}'
  fi
}

usage() {
  cat <<'EOF'
用法:
  bash scripts/jdc_manager.sh install --install-dir /opt/newapi-jdc --data-dir /data/newapi-jdc --port 3000
  bash scripts/jdc_manager.sh start|stop|restart|status
  bash scripts/jdc_manager.sh backup
  bash scripts/jdc_manager.sh restore [备份文件路径|latest]
  bash scripts/jdc_manager.sh clear-db
  bash scripts/jdc_manager.sh uninstall
EOF
}

require_root() {
  if [[ ${EUID} -ne 0 ]]; then
    echo "请用 root 运行此脚本"
    exit 1
  fi
}

load_config() {
  if [[ ! -f "${CONFIG_FILE}" ]]; then
    echo "未找到 ${CONFIG_FILE}，请先执行 install"
    exit 1
  fi
  # shellcheck disable=SC1090
  source "${CONFIG_FILE}"
}

detect_pm() {
  if command -v apt-get >/dev/null 2>&1; then
    echo apt
  elif command -v dnf >/dev/null 2>&1; then
    echo dnf
  elif command -v yum >/dev/null 2>&1; then
    echo yum
  else
    echo unknown
  fi
}

install_deps() {
  local pm
  pm="$(detect_pm)"
  case "${pm}" in
    apt)
      apt-get update
      DEBIAN_FRONTEND=noninteractive apt-get install -y curl tar git golang nodejs npm
      ;;
    dnf)
      dnf install -y curl tar git golang nodejs npm
      ;;
    yum)
      yum install -y curl tar git golang nodejs npm
      ;;
    *)
      echo "无法识别系统包管理器，请手动安装 curl tar git golang nodejs npm"
      exit 1
      ;;
  esac
}

write_service() {
  cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=NewAPI JDC Service
After=network.target

[Service]
Type=simple
WorkingDirectory=${APP_DIR}
EnvironmentFile=${APP_DIR}/.env
ExecStart=${APP_DIR}/newapi --port ${PORT} --log-dir ${LOG_DIR}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
}

write_runtime_files() {
  mkdir -p "${DATA_DIR}" "${LOG_DIR}" "${BACKUP_DIR}" "${INSTALL_DIR}"
  cat > "${APP_DIR}/.env" <<EOF
SESSION_SECRET=${SESSION_SECRET}
CRYPTO_SECRET=${CRYPTO_SECRET}
SQLITE_PATH=${DB_PATH}
EOF
  cat > "${CONFIG_FILE}" <<EOF
INSTALL_DIR=${INSTALL_DIR}
APP_DIR=${APP_DIR}
DATA_DIR=${DATA_DIR}
LOG_DIR=${LOG_DIR}
BACKUP_DIR=${BACKUP_DIR}
DB_PATH=${DB_PATH}
PORT=${PORT}
EOF
}

copy_source() {
  mkdir -p "${APP_DIR}"
  cp -R "${PACKAGE_ROOT}/." "${APP_DIR}/"
  rm -rf "${APP_DIR}/.git" "${APP_DIR}/dist" "${APP_DIR}/web/node_modules" "${APP_DIR}/node_modules"
}

build_app() {
  (cd "${APP_DIR}/web" && npm install --legacy-peer-deps && npm run build)
  (cd "${APP_DIR}" && go build -o newapi .)
}

cmd_install() {
  require_root
  local install_dir="/opt/newapi-jdc"
  local data_dir="/data/newapi-jdc"
  local port="3000"
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --install-dir)
        install_dir="$2"
        shift 2
        ;;
      --data-dir)
        data_dir="$2"
        shift 2
        ;;
      --port)
        port="$2"
        shift 2
        ;;
      *)
        echo "未知参数: $1"
        usage
        exit 1
        ;;
    esac
  done

  install_deps

  INSTALL_DIR="${install_dir}"
  APP_DIR="${INSTALL_DIR}/app"
  DATA_DIR="${data_dir}"
  LOG_DIR="${DATA_DIR}/logs"
  BACKUP_DIR="${DATA_DIR}/backups"
  DB_PATH="${DATA_DIR}/one-api.db"
  PORT="${port}"
  SESSION_SECRET="$(random_secret)"
  CRYPTO_SECRET="$(random_secret)"

  copy_source
  write_runtime_files
  build_app
  write_service

  systemctl daemon-reload
  systemctl enable "${SERVICE_NAME}"
  systemctl restart "${SERVICE_NAME}"

  echo "安装完成"
  echo "安装目录: ${APP_DIR}"
  echo "数据目录: ${DATA_DIR}"
  echo "数据库: ${DB_PATH}"
  echo "端口: ${PORT}"
}

cmd_start() {
  require_root
  load_config
  systemctl start "${SERVICE_NAME}"
}

cmd_stop() {
  require_root
  load_config
  systemctl stop "${SERVICE_NAME}"
}

cmd_restart() {
  require_root
  load_config
  systemctl restart "${SERVICE_NAME}"
}

cmd_status() {
  require_root
  load_config
  systemctl status "${SERVICE_NAME}" --no-pager
}

cmd_backup() {
  require_root
  load_config
  mkdir -p "${BACKUP_DIR}"
  if [[ ! -f "${DB_PATH}" ]]; then
    echo "数据库不存在: ${DB_PATH}"
    exit 1
  fi
  local ts backup_file
  ts="$(date +%Y%m%d-%H%M%S)"
  backup_file="${BACKUP_DIR}/one-api-${ts}.db"
  cp "${DB_PATH}" "${backup_file}"
  [[ -f "${DB_PATH}-wal" ]] && cp "${DB_PATH}-wal" "${backup_file}-wal" || true
  [[ -f "${DB_PATH}-shm" ]] && cp "${DB_PATH}-shm" "${backup_file}-shm" || true
  echo "备份完成: ${backup_file}"
}

cmd_restore() {
  require_root
  load_config
  local target="${1:-latest}"
  if [[ "${target}" == "latest" ]]; then
    target="$(ls -1t "${BACKUP_DIR}"/*.db 2>/dev/null | head -n 1 || true)"
  fi
  if [[ -z "${target}" || ! -f "${target}" ]]; then
    echo "未找到备份文件"
    exit 1
  fi
  systemctl stop "${SERVICE_NAME}"
  cp "${target}" "${DB_PATH}"
  systemctl start "${SERVICE_NAME}"
  echo "还原完成: ${target}"
}

cmd_clear_db() {
  require_root
  load_config
  if [[ -f "${DB_PATH}" ]]; then
    cmd_backup
  fi
  systemctl stop "${SERVICE_NAME}"
  rm -f "${DB_PATH}" "${DB_PATH}-wal" "${DB_PATH}-shm"
  systemctl start "${SERVICE_NAME}"
  echo "数据库已清空并重建，系统恢复到刚安装状态"
}

cmd_uninstall() {
  require_root
  load_config
  systemctl stop "${SERVICE_NAME}" || true
  systemctl disable "${SERVICE_NAME}" || true
  rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
  systemctl daemon-reload
  rm -rf "${APP_DIR}"
  rm -f "${CONFIG_FILE}"
  echo "已卸载程序，数据库和日志保留在 ${DATA_DIR}"
}

main() {
  local cmd="${1:-}"
  case "${cmd}" in
    install)
      shift
      cmd_install "$@"
      ;;
    start)
      cmd_start
      ;;
    stop)
      cmd_stop
      ;;
    restart)
      cmd_restart
      ;;
    status)
      cmd_status
      ;;
    backup)
      cmd_backup
      ;;
    restore)
      shift || true
      cmd_restore "$@"
      ;;
    clear-db)
      cmd_clear_db
      ;;
    uninstall)
      cmd_uninstall
      ;;
    *)
      usage
      exit 1
      ;;
  esac
}

main "$@"
