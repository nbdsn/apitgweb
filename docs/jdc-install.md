# JDC 安装与管理

## GitHub 一键入口

全新服务器直接运行：

```bash
sudo bash <(curl -fsSL https://raw.githubusercontent.com/nbdsn/apitgweb/codex-jdc-backup-tg/scripts/install_from_github.sh)
```

说明：

- 第一次运行时会进入交互式安装向导
- 安装目录、数据库目录、端口都支持直接回车使用默认值
- 安装完成后，再运行同一条命令会进入交互式管理菜单
- 安装器默认优先下载 GitHub Releases 中的预编译安装包
- 如果预编译安装包暂时不可用，会自动回退到源码构建

如果需要非交互安装：

```bash
sudo bash <(curl -fsSL https://raw.githubusercontent.com/nbdsn/apitgweb/codex-jdc-backup-tg/scripts/install_from_github.sh) install --install-dir /opt/newapi-jdc --data-dir /data/newapi-jdc --port 3000
```

## 包内管理脚本

源码包或安装后的程序内，也可以直接执行：

```bash
bash scripts/jdc_manager.sh
```

安装完成后，也支持以下命令模式：

```bash
bash scripts/jdc_manager.sh start
bash scripts/jdc_manager.sh stop
bash scripts/jdc_manager.sh restart
bash scripts/jdc_manager.sh status
bash scripts/jdc_manager.sh backup
bash scripts/jdc_manager.sh restore latest
bash scripts/jdc_manager.sh clear-db
bash scripts/jdc_manager.sh uninstall
```

## Ubuntu / Debian

```bash
sudo bash scripts/jdc_manager.sh
```

## CentOS / Rocky / AlmaLinux

```bash
sudo bash scripts/jdc_manager.sh
```

## 行为说明

- `backup`：复制 SQLite 数据库到 `${DATA_DIR}/backups`
- `restore`：从指定备份或最新备份恢复数据库
- `clear-db`：先备份，再删除当前数据库，服务重启后会生成全新库
- `uninstall`：删除程序与 systemd 服务，但保留数据库和日志
- 数据库默认在 `${DATA_DIR}/one-api.db`
- 日志默认在 `${DATA_DIR}/logs`

## 打包

在源码目录执行：

```bash
bash scripts/package_jdc_release.sh
```

会在 `dist/` 下生成一个可上传到 TG 的源码安装包。

如果要在本地生成预编译安装包：

```bash
bash scripts/package_jdc_prebuilt_release.sh
```

默认会在 `dist/` 下生成 `newapi-jdc-linux-amd64.tar.gz`。
