# Deploying on the Forke OCI VM

Forke Mail runs on the Always Free Ampere VM as the `forke-mail` systemd service on `127.0.0.1:8085`, next to the PM2 apps, behind nginx at `https://mail.forke.space`. It uses its own `forkemail` database and reads Forke's `admins` table through the read-only role `forkemail_admins_reader`. `admins` has row-level security enabled, so that role also needs the `forke_mail_admin_lookup` SELECT policy, or it sees no rows and every login fails with `invalid_credentials`.

## Release a new version

```sh
(cd frontend && npm ci && npm run build)
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o forke-mail-linux-arm64 ./cmd/server
scp forke-mail-linux-arm64 deploy/forke-mail.service ubuntu@<vm>:/tmp/
ssh ubuntu@<vm> 'bash -s' < deploy/install.sh
```

`install.sh` is idempotent:
- it installs the binary and unit file, then restarts the service and prints the health check;
- on the first run only, it creates the `forkemail` user, database and roles, and writes `/opt/forke/forke-mail/.env.production` (root:forkemail, 640) with generated secrets and `SENDER_DRIVER=log`.

Migrations run on start.

## nginx

`mail.forke.space.conf` is installed as `/etc/nginx/sites-available/forke-mail` and enabled. It reuses the shared `forke.space` certificate. After the DNS record exists, add `mail.forke.space` to that certificate:

```sh
sudo certbot certonly --nginx --cert-name forke.space --expand -d forke.space -d www.forke.space -d admin.forke.space -d dashboard.forke.space -d mail.forke.space
sudo nginx -t && sudo systemctl reload nginx
```

## OCI Email Delivery

- Email domain `forke.space` in `ap-mumbai-1`, with the DKIM selector `forkemail-mumbai-202611`.
- Approved senders: `test`, `waitlist`, `onboarding`, `blog`, `changelog`, `approvals`, `admin`, `bans`, `security`, `db` and `support` at `forke.space`.
- SMTP credentials belong to the OCI API user and live only in the env file.
- `SMTP_DAILY_LIMIT=100` and `SMTP_MONTHLY_LIMIT=3000` keep sending inside the free allowance.

## DNS (Cloudflare)

| Type | Name | Value |
|---|---|---|
| A (proxied) | `mail` | VM public IP |
| CNAME (DNS only) | `forkemail-mumbai-202611._domainkey` | `forkemail-mumbai-202611.forke.space.dkim.bom1.oracleemaildelivery.com` |
| TXT | `@` | `v=spf1 include:_spf.google.com include:ap.rp.oracleemaildelivery.com ~all` |

## Operations

```sh
sudo systemctl status forke-mail
sudo journalctl -u forke-mail -f
sudo -u postgres psql forkemail
```
