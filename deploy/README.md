# Deploying on the Forke OCI VM

Forke Mail runs as one binary next to the existing PM2 apps, behind nginx.

1. Build for the Ampere (arm64) VM, with the dashboard embedded:

   ```sh
   (cd frontend && npm ci && npm run build)
   GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o forke-mail ./cmd/server
   ```

2. On the VM, create a database and a read-only role for Forke's admins table:

   ```sql
   CREATE DATABASE forkemail;
   CREATE ROLE forkemail_admins_reader LOGIN PASSWORD '...';
   GRANT CONNECT ON DATABASE forkedb TO forkemail_admins_reader;
   \c forkedb
   GRANT USAGE ON SCHEMA public TO forkemail_admins_reader;
   GRANT SELECT (id, email, username, name, password_hash, role, is_disabled) ON admins TO forkemail_admins_reader;
   ```

3. Copy `forke-mail` to `/opt/forke/forke-mail/` and write `/opt/forke/forke-mail/.env.production` (mode 600) from `.env.example`:
   - `APP_ENV=production`
   - `HTTP_ADDR=127.0.0.1:8085`
   - `PUBLIC_BASE_URL=https://mail.forke.space`
   - `TRUSTED_PROXY=127.0.0.1`
   - `DATABASE_URL` and `FORKE_DATABASE_URL` (the read-only role above)
   - `UNSUBSCRIBE_SECRET` from `openssl rand -hex 32`, and `WEBHOOK_ENCRYPTION_KEY` from `openssl rand -base64 32`
   - the `SMTP_*` values for OCI Email Delivery

4. Install the unit and the site:

   ```sh
   sudo useradd --system --no-create-home forkemail
   sudo cp deploy/forke-mail.service /etc/systemd/system/
   sudo systemctl daemon-reload && sudo systemctl enable --now forke-mail
   sudo cp deploy/mail.forke.space.conf /etc/nginx/sites-available/ && sudo ln -s ../sites-available/mail.forke.space.conf /etc/nginx/sites-enabled/
   sudo certbot certonly --nginx -d mail.forke.space && sudo nginx -t && sudo systemctl reload nginx
   ```

5. Add a DNS record for `mail.forke.space` pointing at the VM in Cloudflare, then check `https://mail.forke.space/healthz`.

If the Cloudflare record is proxied (orange cloud), add nginx `set_real_ip_from` lines for Cloudflare's IP ranges with `real_ip_header CF-Connecting-IP;` so per-IP rate limits see real visitors.
