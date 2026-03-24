# JDC 安装与管理

## 一键管理脚本

包内统一使用：

```bash
bash scripts/jdc_manager.sh install --install-dir /opt/newapi-jdc --data-dir /data/newapi-jdc --port 3000
```

安装完成后，可重复执行以下命令：

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
sudo bash scripts/jdc_manager.sh install --install-dir /opt/newapi-jdc --data-dir /data/newapi-jdc --port 3000
```

## CentOS / Rocky / AlmaLinux

```bash
sudo bash scripts/jdc_manager.sh install --install-dir /opt/newapi-jdc --data-dir /data/newapi-jdc --port 3000
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
