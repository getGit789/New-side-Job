# Installation guide

BriefRelay is one static binary. It needs no runtime, no database server and no PHP.
A first install takes about 15 minutes, most of it DNS and TLS.

## Supported environment

| Item | Requirement |
|---|---|
| Server | Linux x86-64 or ARM64 (Debian 12, Ubuntu 22.04/24.04, or any distribution with glibc or musl). 1 vCPU, 512 MB RAM, 1 GB disk plus your uploads. |
| Network | One port for the app (default 8080, localhost only) and a reverse proxy that terminates HTTPS. |
| Reverse proxy | Caddy 2 (example included) or nginx 1.20+. Any proxy that sets `X-Forwarded-For` works. |
| Browsers | Current and previous major version of Chrome, Firefox, Safari, Edge. |
| Email | Any SMTP server or provider (STARTTLS on 587 or TLS on 465). Optional, but clients get no emails without it. |
| Not supported | Shared PHP hosting without shell access, Windows servers (use Docker there). |

Docker is also supported: `docker compose up -d` with the included compose file.

## Steps

1. **Unpack** the archive into `/opt/briefrelay` and create a system user:
   ```bash
   sudo useradd --system --home /opt/briefrelay --shell /usr/sbin/nologin briefrelay
   sudo mkdir -p /opt/briefrelay && sudo tar -xzf briefrelay-*-linux-amd64.tar.gz --strip-components=1 -C /opt/briefrelay
   sudo mkdir -p /opt/briefrelay/data && sudo chown -R briefrelay:briefrelay /opt/briefrelay
   ```
2. **Configure**: `cp .env.example .env` and set at least:
   - `BRIEFRELAY_BASE_URL=https://portal.yourdomain.com` (the public address; links in emails use it)
   - `BRIEFRELAY_TRUST_PROXY=true` (because a proxy is in front)
   - the `BRIEFRELAY_SMTP_*` values from your email provider
   Keep `BRIEFRELAY_ENV=production`. Production refuses to start without an https base URL.
3. **Preflight**:
   ```bash
   cd /opt/briefrelay && sudo -u briefrelay ./briefrelay check
   ```
   It must end with `Preflight passed.` Warnings about mail are fine if you skipped SMTP for now.
4. **Run as a service**: copy `deploy/briefrelay.service` to `/etc/systemd/system/`, then
   ```bash
   sudo systemctl enable --now briefrelay
   sudo systemctl status briefrelay
   ```
5. **HTTPS proxy**: copy `deploy/Caddyfile` to `/etc/caddy/Caddyfile`, put your host name in, `sudo systemctl reload caddy`.
   Caddy obtains the certificate. For nginx, proxy `/` to `127.0.0.1:8080` with `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;` and `client_max_body_size 60m;`.
6. **Create the owner**: open `https://portal.yourdomain.com/setup` in a browser. The page disappears after the first owner exists.
7. **Settings** (top bar, owner only): workspace name, logo, contact details shown to clients, time zone, date format,
   default currency, and the list of allowed upload types. Everything else is in `.env`.

Optional sample data for a look around: `./briefrelay seed` on an empty install. The sample accounts and their passwords are printed; change or delete them before real use.

## Permissions and scheduled jobs

- The binary needs read/write on `data/` only. Nothing under the web root, nothing executable in `data/`.
- There is no cron to set up. The server runs its own scheduler: email delivery with retries, session and job cleanup, purge of deleted files. If the process is not running, nothing runs; systemd restarts it on failure.
- Add a daily cron for backups: `0 3 * * * /opt/briefrelay/briefrelay backup /backups/briefrelay-$(date +\%F).tar.gz` (see `docs/operations.md`).

## Email and file storage

- Files live in `data/files/` with random names and are served only through authorized downloads. Keep `data/` on the same disk as the database; back them up together with the `backup` command.
- Mail: leave `BRIEFRELAY_SMTP_HOST` empty to log emails instead of sending (useful on a test server). Failed sends retry with back-off up to 5 times; the queue state is in `/healthz`.

## Production security checklist

- [ ] `BRIEFRELAY_ENV=production` and an https base URL.
- [ ] Port 8080 bound to `127.0.0.1` (default) and reachable only through the proxy.
- [ ] `BRIEFRELAY_TRUST_PROXY=true` only when a proxy is in front (otherwise rate limits can be spoofed).
- [ ] `.env` readable by the service user only: `chmod 600 .env`.
- [ ] Daily backups off the machine; a restore tested once (`docs/operations.md`).
- [ ] Owner password at least 12 characters; staff and clients get single-use invitations, never shared passwords.
- [ ] Operating system updates on a schedule.

## Diagnosing failed jobs and mail

- `curl -s http://127.0.0.1:8080/healthz` shows `checks` and `jobs` counts (`pending`, `dead`). `mail` says `not configured` when SMTP is empty.
- `journalctl -u briefrelay` shows JSON log lines. Search for `"level":"ERROR"` and the `last_error` of mail jobs.
- Common causes: wrong SMTP port for the TLS mode (587 = STARTTLS, 465 = TLS), a provider that needs an app password, or `BRIEFRELAY_MAIL_FROM` not allowed by the provider.

## Asking for support

Include: BriefRelay version (`./briefrelay version`), the output of `./briefrelay check`, the `/healthz` JSON,
the relevant log lines with the `id` of the failing request, your OS and proxy. Never send `.env`, the database,
uploaded files, or passwords. Support covers installation and defects in the documented environment; custom
development and server administration are outside the scope.
