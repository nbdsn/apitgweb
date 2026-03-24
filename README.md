# NewAPI JDC Edition

This repository provides a JDC-focused build of NewAPI with:
- Interactive one-command installer
- Daily database backup / import / restore
- JDC Telegram admin bot (interactive menus)
- Prebuilt release pipeline for fast deployment

## 1) One-command install (interactive)

Run directly on Ubuntu / Debian / CentOS:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/nbdsn/apitgweb/codex-jdc-backup-tg/scripts/install_from_github.sh)
```

Behavior:
- First run: opens install wizard
- Press Enter to accept defaults for install path / data path / port
- Run same command again: opens management menu

## 2) Management commands

After install, management script is:

```bash
bash /opt/newapi-jdc/app/scripts/jdc_manager.sh
```

Supported operations:
- Start / Stop / Restart / Status
- Backup DB
- Restore DB
- Clear DB (reset app data)
- Uninstall app (keeps DB and logs)

## 3) Paths (default)

- Install dir: `/opt/newapi-jdc`
- App dir: `/opt/newapi-jdc/app`
- Data dir: `/data/newapi-jdc`
- DB file: `/data/newapi-jdc/one-api.db`
- Logs dir: `/data/newapi-jdc/logs`
- Backups dir: `/data/newapi-jdc/backups`

## 4) JDC web settings pages

- `jdc 备份`
- `jdcTG`

`jdc 备份`:
- Scheduled backup time
- Backup retention / cleanup
- Export / import / restore

`jdcTG`:
- TG bot token
- Admin Telegram IDs (numeric IDs)
- Daily quota adjustment switch and thresholds
- Whitelist and daily report

## 5) Telegram bot commands

The bot auto-registers slash commands via `setMyCommands`:
- `/start`
- `/help`
- `/stats`
- `/users`
- `/user`
- `/addquota`
- `/subquota`
- `/redeem`

Highlights:
- `/stats`: detailed usage / quota / token / average RPM / performance metrics
- `/users`: interactive user menu (lowest quota top 10), then:
  - add quota
  - reduce quota
  - enable account
  - disable account
- `/redeem`: interactive generation (amount -> count)

## 6) Fast install with prebuilt bundle

Installer first downloads prebuilt release artifact:
- Release tag: `jdc-latest`
- Asset: `newapi-jdc-linux-amd64.tar.gz`

If prebuilt is unavailable, installer falls back to source build.

## 7) GitHub Actions

- Prebuilt bundle workflow:
  - `.github/workflows/jdc-prebuilt-release.yml`
- Docker workflow (GHCR):
  - `.github/workflows/jdc-docker-ghcr.yml`

## 8) Docker image

Workflow publishes image to GHCR:
- `ghcr.io/nbdsn/apitgweb:jdc-latest`
- `ghcr.io/nbdsn/apitgweb:jdc-<short_sha>`

Example run:

```bash
docker run -d --name newapi-jdc \
  -p 3000:3000 \
  -v /data/newapi-jdc:/data \
  ghcr.io/nbdsn/apitgweb:jdc-latest
```

## 9) Notes

- Use numeric Telegram user IDs in `jdcTG` admin list (not `@username`)
- Start a chat with your bot (`/start`) before testing push messages
- DB and logs are preserved on uninstall by design
