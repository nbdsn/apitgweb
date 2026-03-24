# NewAPI JDC 版本

这个仓库是面向 JDC 的 NewAPI 版本，核心能力：
- 一键交互安装
- 数据库定时备份 / 导入 / 还原
- JDC Telegram 管理机器人（交互菜单）
- GitHub 预编译发布，快速部署

## 1）一键安装（交互式）

在 Ubuntu / Debian / CentOS 服务器直接执行：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/nbdsn/apitgweb/codex-jdc-backup-tg/scripts/install_from_github.sh)
```

行为：
- 第一次运行：进入安装向导
- 安装目录、数据目录、端口可以直接回车用默认值
- 再次运行同一命令：进入管理菜单

## 2）管理命令

安装后管理脚本路径：

```bash
bash /opt/newapi-jdc/app/scripts/jdc_manager.sh
```

支持：
- 启动 / 停止 / 重启 / 状态
- 备份数据库
- 还原数据库
- 清空数据库（恢复初始）
- 卸载程序（保留数据库和日志）

## 3）默认目录

- 安装目录：`/opt/newapi-jdc`
- 程序目录：`/opt/newapi-jdc/app`
- 数据目录：`/data/newapi-jdc`
- 数据库：`/data/newapi-jdc/one-api.db`
- 日志目录：`/data/newapi-jdc/logs`
- 备份目录：`/data/newapi-jdc/backups`

## 4）Web 设置菜单

新增两个设置页：
- `jdc 备份`
- `jdcTG`

`jdc 备份` 支持：
- 定时备份时间
- 备份保留 / 清理
- 导出 / 导入 / 还原

`jdcTG` 支持：
- Bot Token
- 管理员 Telegram ID（数字ID）
- 每日额度调整开关与阈值
- 白名单与日报开关

## 5）TG 机器人命令

机器人会自动注册 slash 命令（`setMyCommands`）：
- `/start`
- `/help`
- `/stats`
- `/users`
- `/user`
- `/addquota`
- `/subquota`
- `/redeem`

重点能力：
- `/stats`：详细展示请求次数、统计次数、统计额度、统计 Tokens、平均 RPM、系统性能
- `/users`：显示额度最低前 10 用户并进入交互菜单，可继续执行：
  - 增加额度
  - 减少额度
  - 启用账户
  - 停用账户
- `/redeem`：交互式生成兑换码（先选金额，再选张数）

## 6）快速安装（预编译包）

安装器默认优先下载预编译发布包：
- Release 标签：`jdc-latest`
- 文件：`newapi-jdc-linux-amd64.tar.gz`

如果预编译包暂不可用，会自动回退源码构建。

## 7）GitHub Actions

- 预编译打包工作流：
  - `.github/workflows/jdc-prebuilt-release.yml`
- Docker 构建工作流（GHCR）：
  - `.github/workflows/jdc-docker-ghcr.yml`

## 8）Docker 镜像

工作流会发布到 GHCR：
- `ghcr.io/nbdsn/apitgweb:jdc-latest`
- `ghcr.io/nbdsn/apitgweb:jdc-<short_sha>`

运行示例：

```bash
docker run -d --name newapi-jdc \
  -p 3000:3000 \
  -v /data/newapi-jdc:/data \
  ghcr.io/nbdsn/apitgweb:jdc-latest
```

## 9）注意事项

- `jdcTG` 里管理员必须填 Telegram 数字用户 ID，不是 `@用户名`
- 管理员需先和 Bot 私聊发送一次 `/start`
- 卸载流程默认保留数据库与日志
