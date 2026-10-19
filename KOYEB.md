# Deploying TG-FileStreamBot on Koyeb

## 1. Prerequisites
| What | Where to get it |
|---|---|
| `API_ID` and `API_HASH` | https://my.telegram.org -> API development tools |
| `BOT_TOKEN` | Telegram @BotFather -> `/newbot` |
| `LOG_CHANNEL` | Create a **private Telegram channel**, add your bot as **Admin**, then forward any message from the channel to @userinfobot / @JsonDumpBot (or @MissRose_bot `/id`) to read its ID. It looks like `-1001234567890`. |
| GitHub account | To host this code (Koyeb builds from the Dockerfile) |
| Koyeb account | https://app.koyeb.com |

## 2. Push the code to GitHub
```bash
cd TG-FileStreamBot-main
git init && git add . && git commit -m "Koyeb ready"
git branch -M main
git remote add origin https://github.com/<you>/<repo>.git
git push -u origin main
```
(`fsb.env`, `.env` and session files are git-ignored, so no secrets get pushed.)

## 3. Create the Koyeb service
1. Koyeb dashboard -> **Create Service** -> **GitHub** -> pick your repo, branch `main`.
2. **Builder:** choose **Dockerfile** (leave the path as `Dockerfile`).
3. **Instance:** Free (or any paid size). 512 MB RAM is enough.
4. **Service type:** **Web service**.
5. **Environment variables:** add the variables from section 4 (store `API_HASH`, `BOT_TOKEN`, `USER_SESSION`, `MULTI_TOKEN*` as **Secrets**).
6. **Exposed ports:** `8000`, protocol **HTTP**, path `/`.
7. **Health check:** HTTP, port `8000`, path `/` (the app returns `{"ok":true,...}` there).
8. Name the service and click **Deploy**.

## 4. Environment variables

### Required
| Variable | Example | Notes |
|---|---|---|
| `API_ID` | `1234567` | From my.telegram.org |
| `API_HASH` | `0123456789abcdef0123456789abcdef` | From my.telegram.org (secret) |
| `BOT_TOKEN` | `123456:ABC-DEF...` | From @BotFather (secret) |
| `LOG_CHANNEL` | `-1001234567890` | Bot must be admin in this channel |
| `PORT` | `8000` | Must match the exposed port. Already the Docker default, but set it explicitly. |

### Recommended
| Variable | Value | Notes |
|---|---|---|
| `HOST` | `https://<app>-<org>.koyeb.app` | Public URL used in generated links. **Optional on Koyeb**: if unset, the app auto-uses Koyeb's `KOYEB_PUBLIC_DOMAIN`. Set it only for a custom domain. No trailing slash. |
| `USE_SESSION_FILE` | `false` | Koyeb's disk is ephemeral, so session files are lost on redeploy anyway. `false` avoids needless writes. |

### Optional
| Variable | Default | Notes |
|---|---|---|
| `HASH_LENGTH` | `6` | 5-32; length of the `?hash=` in links |
| `ALLOWED_USERS` | empty (everyone) | Comma-separated Telegram user IDs allowed to use the bot, e.g. `12345,67890` |
| `USER_SESSION` | empty | Pyrogram session string; lets a userbot auto-add worker bots to the log channel |
| `MULTI_TOKEN1`, `MULTI_TOKEN2`, ... | empty | Extra bot tokens (add each as admin to the log channel) to spread load and speed up streaming |
| `STREAM_CONCURRENCY` | `4` | Parallel chunk downloads per request |
| `STREAM_BUFFER_COUNT` | `8` | Chunks buffered in memory (lower to `4` on the 512 MB free instance if you see OOM) |
| `STREAM_TIMEOUT_SEC` | `30` | Per-chunk timeout |
| `STREAM_MAX_RETRIES` | `3` | Retries per failed chunk |
| `DEV` | `false` | Debug logging |
| `USE_PUBLIC_IP` | `false` | Leave off on Koyeb |

## 5. Verify
1. Wait for the deployment to turn **Healthy**; check the **Runtime logs** for `Server is running at https://...koyeb.app`.
2. Open `https://<your-app>.koyeb.app/`: you should see `{"message":"Server is running.","ok":true,...}`.
3. Send `/start` to your bot, then send it any file/video. It replies with a stream link of the form `https://<your-app>.koyeb.app/stream/<id>?hash=<hash>`.

## 6. Troubleshooting
- **Health check fails / "port not reachable":** make sure `PORT=8000` and the exposed port is 8000/HTTP.
- **Bot doesn't answer:** check the logs for `Failed to start main bot`; re-check `API_ID`, `API_HASH`, `BOT_TOKEN`.
- **"Chat not found" / no link generated:** `LOG_CHANNEL` is wrong (needs the `-100` prefix form) or the bot is not an admin of that channel.
- **Links show the wrong domain:** set `HOST` explicitly.
- **Out of memory:** lower `STREAM_BUFFER_COUNT` / `STREAM_CONCURRENCY`, or use a bigger instance.

## 7. Free-tier note: keep the bot awake
Koyeb free instances scale to zero after a period without incoming HTTP traffic, and a sleeping bot stops receiving Telegram messages. Either:
- ping `https://<your-app>.koyeb.app/` every ~5 minutes with a free monitor (UptimeRobot, cron-job.org), or
- use a paid instance type that doesn't sleep.

Free-tier limits and policies change, so confirm on koyeb.com/pricing. Also note that streaming large files uses bandwidth, which is quota-limited on every plan.
