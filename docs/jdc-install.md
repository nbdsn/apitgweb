# JDC Install Guide

## Interactive install

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/nbdsn/apitgweb/codex-jdc-backup-tg/scripts/install_from_github.sh)
```

## Defaults

- Install: `/opt/newapi-jdc`
- Data: `/data/newapi-jdc`
- Port: `3000`

## Management

```bash
bash /opt/newapi-jdc/app/scripts/jdc_manager.sh
```

## Important commands

```bash
bash /opt/newapi-jdc/app/scripts/jdc_manager.sh start
bash /opt/newapi-jdc/app/scripts/jdc_manager.sh stop
bash /opt/newapi-jdc/app/scripts/jdc_manager.sh restart
bash /opt/newapi-jdc/app/scripts/jdc_manager.sh status
bash /opt/newapi-jdc/app/scripts/jdc_manager.sh backup
bash /opt/newapi-jdc/app/scripts/jdc_manager.sh restore latest
bash /opt/newapi-jdc/app/scripts/jdc_manager.sh clear-db
bash /opt/newapi-jdc/app/scripts/jdc_manager.sh uninstall
```

## Data safety

- `uninstall` keeps DB and logs
- `clear-db` resets app DB to initial state

## Telegram reminders

- Admin IDs must be numeric Telegram IDs
- Admin must send `/start` to bot before test push
