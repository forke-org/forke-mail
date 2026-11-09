#!/usr/bin/env bash
set -euo pipefail
APP=/opt/forke/forke-mail
id forkemail >/dev/null 2>&1 || sudo useradd --system --no-create-home --shell /usr/sbin/nologin forkemail
sudo mkdir -p $APP
sudo install -m 755 -o root -g root /tmp/forke-mail-linux-arm64 $APP/forke-mail
sudo install -m 644 -o root -g root /tmp/forke-mail.service /etc/systemd/system/forke-mail.service
rm -f /tmp/forke-mail-linux-arm64 /tmp/forke-mail.service
psqlq() { sudo -u postgres psql -v ON_ERROR_STOP=1 -q "$@"; }
if [ ! -f $APP/.env.production ]; then
  DBPASS=$(openssl rand -hex 24); READPASS=$(openssl rand -hex 24)
  psqlq -c "CREATE ROLE forkemail LOGIN PASSWORD '$DBPASS'"
  psqlq -c "CREATE DATABASE forkemail OWNER forkemail"
  psqlq -c "CREATE ROLE forkemail_admins_reader LOGIN PASSWORD '$READPASS'"
  psqlq -c "GRANT CONNECT ON DATABASE forkedb TO forkemail_admins_reader"
  psqlq -d forkedb -c "GRANT USAGE ON SCHEMA public TO forkemail_admins_reader"
  psqlq -d forkedb -c "GRANT SELECT (id, email, username, name, password_hash, role, is_disabled) ON public.admins TO forkemail_admins_reader"
  psqlq -d forkedb -c "CREATE POLICY forke_mail_admin_lookup ON public.admins FOR SELECT TO forkemail_admins_reader USING (true)"
  umask 077
  cat > /tmp/forke-mail.env <<ENV
APP_ENV=production
HTTP_ADDR=127.0.0.1:8085
DATABASE_URL=postgres://forkemail:${DBPASS}@127.0.0.1:5432/forkemail?sslmode=disable
FORKE_DATABASE_URL=postgres://forkemail_admins_reader:${READPASS}@127.0.0.1:5432/forkedb?sslmode=disable
FORKE_WORKSPACE_NAME=Forke
PUBLIC_BASE_URL=https://mail.forke.space
TRUSTED_PROXY=127.0.0.1
UNSUBSCRIBE_SECRET=$(openssl rand -hex 32)
WEBHOOK_ENCRYPTION_KEY=$(openssl rand -base64 32)
SENDER_DRIVER=log
SEND_RATE_PER_SEC=2
ENV
  sudo install -m 640 -o root -g forkemail /tmp/forke-mail.env $APP/.env.production
  rm -f /tmp/forke-mail.env
  echo "created env and database roles"
fi
sudo systemctl daemon-reload
sudo systemctl enable forke-mail >/dev/null 2>&1
sudo systemctl restart forke-mail
sleep 3
systemctl is-active forke-mail
curl -s http://127.0.0.1:8085/healthz; echo
sudo journalctl -u forke-mail -n 4 --no-pager -o cat | cut -c1-220
