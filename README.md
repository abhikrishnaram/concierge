# Concierge

A small, self-hosted webhook-and-cron dispatcher. Two ways to run a shell script without SSH access to the host:

1. **Webhook-triggered** — `POST /hooks/{path}` with a matching `X-Deploy-Token` header runs the associated script. Point CI (e.g. GitHub Actions) at it after pushing an image to trigger a redeploy.
2. **Cron-scheduled** — a script runs on a configured cron expression, no external trigger. Replaces ad-hoc host crontabs with one auditable place.

Both are managed from the same admin UI and log to the same run history. Concierge has no opinion on what scripts do — that's entirely up to whatever's configured.

## Run it

```
docker run -d \
  -p 8080:8080 \
  -v concierge-data:/data \
  -e TOTP_ISSUER=MyServer \
  ghcr.io/<user>/concierge:latest
```

| Env var       | Default     | Purpose                                                        |
|---------------|-------------|------------------------------------------------------------------|
| `PORT`        | `8080`      | HTTP listen port                                                |
| `DATA_DIR`    | `/data`     | Holds the SQLite file and the TOTP secret — mount a volume here |
| `TOTP_ISSUER` | `Concierge` | Label shown in the authenticator app                            |

Everything else — routes, cron jobs, scripts — is managed through the admin UI after first boot.

If a script needs tools beyond `/bin/sh` (the docker CLI, `curl`, `python3`, ...), build a derived image rather than expecting the base image to bundle everything:

```dockerfile
FROM ghcr.io/<user>/concierge:latest
RUN apk add --no-cache docker-cli curl
```

Mount whatever the script needs (e.g. the Docker socket) at `docker run` time — Concierge makes no assumptions about the host.

## First login

The admin UI is protected by TOTP (6-digit code, single admin, no password). There's no way to set or reset the TOTP secret over HTTP — that's deliberate: a fully compromised web app still can't take over or lock out admin access. Generate the secret from inside the running container:

```
docker exec -it <container> concierge totp-reset
```

This prints the secret and an `otpauth://` URI — scan it (as a QR code) or enter it manually into an authenticator app (1Password, Google Authenticator, etc.). Run it again any time to rotate the secret.

The admin UI issues a server-side session on login with a hard 2-hour expiry. The session cookie is marked `Secure`, so the admin UI must be served over HTTPS — put a TLS-terminating reverse proxy in front for anything beyond localhost testing. (Webhook triggers themselves don't need this — they're authenticated by the route's own token, not a session.)

## Data model

Single SQLite file under `DATA_DIR`:

- `webhook_routes` — path, name, script, token
- `cron_jobs` — name, schedule, script, enabled
- `runs` — unified execution log for both trigger types
- `sessions` — admin UI logins

## Development

```
go build ./...
go run . totp-reset      # generate a TOTP secret under $DATA_DIR (default /data)
go run .                 # start the server
```

Out of scope for v1: multi-user accounts, RBAC, script version history, a job queue (runs execute synchronously).
