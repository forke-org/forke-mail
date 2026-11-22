# Forke Mail

Email for developers: transactional sends, broadcasts, templates, workflows and SMTP, on Amazon SES.

Live at [mail.forke.space](https://mail.forke.space) · Status at [status.forke.space](https://status.forke.space)

## Run it locally

```sh
docker compose up -d
(cd frontend && npm ci && npm run build)
set -a && . ./.env && set +a && go run ./cmd/server
```

Contributors and coding agents: read [AGENTS.md](AGENTS.md).

## License

Source-available, not open source. See [LICENSE](LICENSE). © 2026 Forke.
