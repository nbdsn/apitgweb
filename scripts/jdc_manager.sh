#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="newapi-jdc"
CONFIG_FILE="/etc/${SERVICE_NAME}.conf"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACKAGE_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DEFAULT_INSTALL_DIR="/opt/newapi-jdc"
DEFAULT_DATA_DIR="/data/newapi-jdc"
DEFAULT_PORT="3000"

if [[ -t 1 ]]; then
  COLOR_RED='\033[31m'
  COLOR_GREEN='\033[32m'
  COLOR_YELLOW='\033[33m'
  COLOR_BLUE='\033[34m'
  COLOR_CYAN='\033[36m'
  COLOR_BOLD='\033[1m'
  COLOR_RESET='\033[0m'
else
  COLOR_RED=''
  COLOR_GREEN=''
  COLOR_YELLOW=''
  COLOR_BLUE=''
  COLOR_CYAN=''
  COLOR_BOLD=''
  COLOR_RESET=''
fi

print_line() {
  printf '%b\n' "${COLOR_BLUE}============================================================${COLOR_RESET}"
}

print_title() {
  print_line
  printf '%b\n' "${COLOR_BOLD}${COLOR_CYAN}$1${COLOR_RESET}"
  print_line
}

info() {
  printf '%b\n' "${COLOR_CYAN}[信息]${COLOR_RESET} $*"
}

success() {
  printf '%b\n' "${COLOR_GREEN}[完成]${COLOR_RESET} $*"
}

warn() {
  printf '%b\n' "${COLOR_YELLOW}[提示]${COLOR_RESET} $*"
}

error() {
  printf '%b\n' "${COLOR_RED}[错误]${COLOR_RESET} $*" >&2
}

pause_screen() {
  if [[ -t 0 ]]; then
    printf '\n'
    read -r -p "按回车继续..." _ || true
  fi
}

random_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 16
  else
    date +%s%N | sha256sum | awk '{print $1}'
  fi
}

usage() {
  cat <<'USAGE'
用法:
  bash scripts/jdc_manager.sh
  bash scripts/jdc_manager.sh install [--install-dir DIR] [--data-dir DIR] [--port PORT]
  bash scripts/jdc_manager.sh start|stop|restart|status
  bash scripts/jdc_manager.sh backup
  bash scripts/jdc_manager.sh restore [备份文件路径|latest]
  bash scripts/jdc_manager.sh clear-db
  bash scripts/jdc_manager.sh uninstall

说明:
  1. 无参数运行时:
     - 未安装: 进入交互式安装向导
     - 已安装: 进入交互式管理菜单
  2. 保留命令行模式，方便自动化脚本调用
USAGE
}

require_root() {
  if [[ ${EUID} -ne 0 ]]; then
    error "请用 root 运行此脚本，或在命令前加 sudo"
    exit 1
  fi
}

config_exists() {
  [[ -f "${CONFIG_FILE}" ]]
}

source_config() {
  if ! config_exists; then
    error "未找到 ${CONFIG_FILE}，请先安装"
    exit 1
  fi
  # shellcheck disable=SC1090
  source "${CONFIG_FILE}"
}

is_installed() {
  if ! config_exists; then
    return 1
  fi

  # shellcheck disable=SC1090
  source "${CONFIG_FILE}"

  [[ -n "${APP_DIR:-}" ]] || return 1
  [[ -x "${APP_DIR}/newapi" ]] || return 1
  [[ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]] || return 1
}

is_partial_install() {
  config_exists && ! is_installed
}

load_config() {
  source_config
}

prompt_with_default() {
  local label="$1"
  local default_value="$2"
  local input=""
  if [[ -t 0 ]]; then
    read -r -p "${label} [${default_value}]: " input || true
  fi
  printf '%s' "${input:-$default_value}"
}

confirm_action() {
  local message="$1"
  local default_answer="${2:-N}"
  local suffix="[y/N]"
  local answer=""

  if [[ "${default_answer}" == "Y" ]]; then
    suffix="[Y/n]"
  fi

  if [[ ! -t 0 ]]; then
    [[ "${default_answer}" == "Y" ]]
    return
  fi

  read -r -p "${message} ${suffix}: " answer || true
  answer="${answer:-$default_answer}"
  case "${answer}" in
    y|Y|yes|YES)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
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

install_runtime_deps() {
  local pm
  pm="$(detect_pm)"
  info "安装运行时依赖..."
  case "${pm}" in
    apt)
      apt-get update
      DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates tzdata
      ;;
    dnf)
      dnf install -y ca-certificates tzdata
      ;;
    yum)
      yum install -y ca-certificates tzdata
      ;;
    *)
      error "无法识别系统包管理器，请手动安装 ca-certificates tzdata"
      exit 1
      ;;
  esac
}

install_build_deps() {
  local pm
  pm="$(detect_pm)"
  install_runtime_deps
  info "安装源码构建依赖..."
  case "${pm}" in
    apt)
      DEBIAN_FRONTEND=noninteractive apt-get install -y curl tar git golang nodejs npm
      ;;
    dnf)
      dnf install -y curl tar git golang nodejs npm
      ;;
    yum)
      yum install -y curl tar git golang nodejs npm
      ;;
    *)
      error "无法识别系统包管理器，请手动安装 curl tar git golang nodejs npm"
      exit 1
      ;;
  esac
}

write_service() {
  cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<SERVICE
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
SERVICE
}

write_runtime_files() {
  mkdir -p "${DATA_DIR}" "${LOG_DIR}" "${BACKUP_DIR}" "${INSTALL_DIR}"

  cat > "${APP_DIR}/.env" <<ENVFILE
SESSION_SECRET=${SESSION_SECRET}
CRYPTO_SECRET=${CRYPTO_SECRET}
SQLITE_PATH=${DB_PATH}
ENVFILE

  cat > "${CONFIG_FILE}" <<CONFIG
INSTALL_DIR=${INSTALL_DIR}
APP_DIR=${APP_DIR}
DATA_DIR=${DATA_DIR}
LOG_DIR=${LOG_DIR}
BACKUP_DIR=${BACKUP_DIR}
DB_PATH=${DB_PATH}
PORT=${PORT}
CONFIG
}

package_has_binary() {
  [[ -x "${PACKAGE_ROOT}/newapi" ]]
}

copy_package_files() {
  mkdir -p "${APP_DIR}"
  cp -R "${PACKAGE_ROOT}/." "${APP_DIR}/"
  rm -rf "${APP_DIR}/.git" "${APP_DIR}/dist" "${APP_DIR}/web/node_modules" "${APP_DIR}/node_modules"
}

build_app() {
  if [[ -x "${APP_DIR}/newapi" ]]; then
    info "检测到预编译安装包，跳过本地构建"
    return
  fi
  info "安装前端依赖并构建页面..."
  (cd "${APP_DIR}/web" && npm install --legacy-peer-deps && npm run build)
  info "编译后端程序..."
  (cd "${APP_DIR}" && go build -o newapi .)
}

ensure_backup_dir() {
  mkdir -p "${BACKUP_DIR}"
}

cmd_install() {
  require_root

  if is_installed; then
    error "检测到系统已经安装。请直接再次运行同一命令进入管理菜单，或先执行 uninstall。"
    exit 1
  fi

  local install_dir="${DEFAULT_INSTALL_DIR}"
  local data_dir="${DEFAULT_DATA_DIR}"
  local port="${DEFAULT_PORT}"

  if is_partial_install; then
    source_config
    install_dir="${INSTALL_DIR:-$install_dir}"
    data_dir="${DATA_DIR:-$data_dir}"
    port="${PORT:-$port}"
    warn "检测到未完成安装，将按修复安装继续执行。"
  fi

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
        error "未知参数: $1"
        usage
        exit 1
        ;;
    esac
  done

  INSTALL_DIR="${install_dir}"
  APP_DIR="${INSTALL_DIR}/app"
  DATA_DIR="${data_dir}"
  LOG_DIR="${DATA_DIR}/logs"
  BACKUP_DIR="${DATA_DIR}/backups"
  DB_PATH="${DATA_DIR}/one-api.db"
  PORT="${port}"
  SESSION_SECRET="$(random_secret)"
  CRYPTO_SECRET="$(random_secret)"

  print_title "NewAPI JDC 安装中"
  info "安装目录: ${APP_DIR}"
  info "数据目录: ${DATA_DIR}"
  info "服务端口: ${PORT}"

  if package_has_binary; then
    install_runtime_deps
  else
    install_build_deps
  fi

  copy_package_files
  build_app
  write_runtime_files
  write_service

  systemctl daemon-reload
  systemctl enable "${SERVICE_NAME}"
  systemctl restart "${SERVICE_NAME}"

  success "安装完成"
  printf '%s\n' "程序目录: ${APP_DIR}"
  printf '%s\n' "数据目录: ${DATA_DIR}"
  printf '%s\n' "数据库文件: ${DB_PATH}"
  printf '%s\n' "日志目录: ${LOG_DIR}"
  printf '%s\n' "访问端口: ${PORT}"
}

cmd_start() {
  require_root
  if ! is_installed; then
    error "当前不是完整安装状态，请重新执行安装向导完成修复。"
    exit 1
  fi
  load_config
  systemctl start "${SERVICE_NAME}"
  success "服务已启动"
}

cmd_stop() {
  require_root
  if ! is_installed; then
    error "当前不是完整安装状态，请先完成修复安装。"
    exit 1
  fi
  load_config
  systemctl stop "${SERVICE_NAME}"
  success "服务已停止"
}

cmd_restart() {
  require_root
  if ! is_installed; then
    error "当前不是完整安装状态，请先完成修复安装。"
    exit 1
  fi
  load_config
  systemctl restart "${SERVICE_NAME}"
  success "服务已重启"
}

cmd_status() {
  require_root
  if ! is_installed; then
    error "当前不是完整安装状态，请先完成修复安装。"
    exit 1
  fi
  load_config
  systemctl status "${SERVICE_NAME}" --no-pager
}

cmd_backup() {
  require_root
  load_config
  ensure_backup_dir

  if [[ ! -f "${DB_PATH}" ]]; then
    error "数据库不存在: ${DB_PATH}"
    exit 1
  fi

  local ts backup_file
  ts="$(date +%Y%m%d-%H%M%S)"
  backup_file="${BACKUP_DIR}/one-api-${ts}.db"

  cp "${DB_PATH}" "${backup_file}"
  [[ -f "${DB_PATH}-wal" ]] && cp "${DB_PATH}-wal" "${backup_file}-wal" || true
  [[ -f "${DB_PATH}-shm" ]] && cp "${DB_PATH}-shm" "${backup_file}-shm" || true

  success "备份完成: ${backup_file}"
}

select_backup_file() {
  local backups=()
  local index choice

  ensure_backup_dir
  while IFS= read -r file; do
    backups+=("${file}")
  done < <(find "${BACKUP_DIR}" -maxdepth 1 -type f -name '*.db' | sort -r)

  if [[ ${#backups[@]} -eq 0 ]]; then
    error "当前没有可用备份"
    return 1
  fi

  print_title "可用备份列表"
  for index in "${!backups[@]}"; do
    printf '  %s) %s\n' "$((index + 1))" "$(basename "${backups[index]}")"
  done
  printf '  0) 取消\n\n'

  while true; do
    read -r -p "请选择要还原的备份编号 [1]: " choice || true
    choice="${choice:-1}"

    if [[ "${choice}" == "0" ]]; then
      return 1
    fi

    if [[ "${choice}" =~ ^[0-9]+$ ]] && (( choice >= 1 && choice <= ${#backups[@]} )); then
      printf '%s' "${backups[choice - 1]}"
      return 0
    fi

    warn "输入无效，请重新选择"
  done
}

restore_backup_file() {
  local target="$1"

  if [[ -z "${target}" || ! -f "${target}" ]]; then
    error "未找到备份文件: ${target}"
    exit 1
  fi

  systemctl stop "${SERVICE_NAME}"
  rm -f "${DB_PATH}" "${DB_PATH}-wal" "${DB_PATH}-shm"
  cp "${target}" "${DB_PATH}"
  [[ -f "${target}-wal" ]] && cp "${target}-wal" "${DB_PATH}-wal" || true
  [[ -f "${target}-shm" ]] && cp "${target}-shm" "${DB_PATH}-shm" || true
  systemctl start "${SERVICE_NAME}"

  success "还原完成: ${target}"
}

cmd_restore() {
  require_root
  load_config

  local target="${1:-latest}"
  if [[ "${target}" == "latest" ]]; then
    target="$(find "${BACKUP_DIR}" -maxdepth 1 -type f -name '*.db' | sort -r | head -n 1 || true)"
  elif [[ -z "${target}" && -t 0 ]]; then
    target="$(select_backup_file)" || return 0
  fi

  if [[ -z "${target}" ]]; then
    error "未找到备份文件"
    exit 1
  fi

  restore_backup_file "${target}"
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

  success "数据库已清空，服务启动后会自动生成全新库"
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

  success "已卸载程序，数据库和日志保留在 ${DATA_DIR}"
}

show_install_wizard() {
  require_root
  print_title "NewAPI JDC 安装向导"
  printf '%s\n' "直接回车即可使用默认值。"
  printf '\n'

  local install_dir data_dir port
  if is_partial_install; then
    source_config
    install_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    data_dir="${DATA_DIR:-$DEFAULT_DATA_DIR}"
    port="${PORT:-$DEFAULT_PORT}"
    warn "检测到当前机器处于半安装状态，本次将执行修复安装。"
  else
    install_dir="${DEFAULT_INSTALL_DIR}"
    data_dir="${DEFAULT_DATA_DIR}"
    port="${DEFAULT_PORT}"
  fi
  install_dir="$(prompt_with_default '安装目录' "${install_dir}")"
  data_dir="$(prompt_with_default '数据库和日志目录' "${data_dir}")"
  port="$(prompt_with_default '服务端口' "${port}")"

  printf '\n'
  info "将按以下配置安装"
  printf '  程序目录: %s/app\n' "${install_dir}"
  printf '  数据目录: %s\n' "${data_dir}"
  printf '  服务端口: %s\n' "${port}"
  printf '\n'

  if ! confirm_action "开始安装吗" "Y"; then
    warn "已取消安装"
    exit 0
  fi

  cmd_install --install-dir "${install_dir}" --data-dir "${data_dir}" --port "${port}"
}

show_manage_menu() {
  require_root
  load_config

  while true; do
    print_title "NewAPI JDC 管理菜单"
    printf '程序目录: %s\n' "${APP_DIR}"
    printf '数据目录: %s\n' "${DATA_DIR}"
    printf '数据库文件: %s\n' "${DB_PATH}"
    printf '服务端口: %s\n' "${PORT}"
    printf '\n'
    printf '  1) 启动服务\n'
    printf '  2) 停止服务\n'
    printf '  3) 重启服务\n'
    printf '  4) 查看状态\n'
    printf '  5) 备份数据库\n'
    printf '  6) 还原数据库\n'
    printf '  7) 清空数据库\n'
    printf '  8) 卸载程序\n'
    printf '  0) 退出\n\n'

    local choice=""
    read -r -p "请选择操作: " choice || true
    printf '\n'

    case "${choice}" in
      1)
        cmd_start
        pause_screen
        ;;
      2)
        cmd_stop
        pause_screen
        ;;
      3)
        cmd_restart
        pause_screen
        ;;
      4)
        cmd_status
        pause_screen
        ;;
      5)
        cmd_backup
        pause_screen
        ;;
      6)
        local selected_backup=""
        selected_backup="$(select_backup_file)" || continue
        if confirm_action "确认从 $(basename "${selected_backup}") 还原数据库吗" "N"; then
          restore_backup_file "${selected_backup}"
        else
          warn "已取消还原"
        fi
        pause_screen
        ;;
      7)
        if confirm_action "确认清空当前数据库并恢复到初装状态吗" "N"; then
          cmd_clear_db
        else
          warn "已取消清空数据库"
        fi
        pause_screen
        ;;
      8)
        if confirm_action "确认卸载程序吗？数据库和日志会保留" "N"; then
          cmd_uninstall
          break
        else
          warn "已取消卸载"
          pause_screen
        fi
        ;;
      0)
        success "已退出管理菜单"
        break
        ;;
      *)
        warn "无效选项，请重新输入"
        pause_screen
        ;;
    esac
  done
}

main() {
  local cmd="${1:-}"

  if [[ -z "${cmd}" ]]; then
    if is_installed; then
      show_manage_menu
    else
      show_install_wizard
    fi
    return
  fi

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
    menu)
      show_manage_menu
      ;;
    help|-h|--help)
      usage
      ;;
    *)
      error "未知命令: ${cmd}"
      usage
      exit 1
      ;;
  esac
}

main "$@"
