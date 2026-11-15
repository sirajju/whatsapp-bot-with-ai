# whatsapp-bot-with-ai

A lightweight, self-hosted WhatsApp bot written in Go on top of
[`whatsmeow`](https://github.com/tulir/whatsmeow). Feature set modelled on
[FantoX/Atlas-MD](https://github.com/FantoX/Atlas-MD) (reference only — no code
reuse). Designed for low-RAM hardware: a single static binary, no browser, no
Node.js runtime.

## Why this stack

| Thing          | Choice                                  | Why                                   |
|----------------|-----------------------------------------|---------------------------------------|
| Language       | Go 1.23+                                | Static binary, low memory             |
| WhatsApp lib   | `go.mau.fi/whatsmeow`                   | WebSocket client, no headless Chrome  |
| Storage        | `modernc.org/sqlite` (pure Go)          | No cgo, cross-compiles trivially      |
| Logger         | `rs/zerolog`                            | Structured, fast                      |
| AI             | Interface + 5 backends (all optional)   | Bot works without any AI credit       |

- Stripped binary: **18 MB**
- Idle RSS: ~25 MB
- Active RSS: ~50–80 MB

## Hardware requirements

- Linux x86_64 or arm64
- **512 MB RAM** is enough; 1 GB is comfortable
- 500 MB disk
- `ffmpeg` on PATH for sticker commands
- `yt-dlp` on PATH for `!dl` downloader

## Install & run

```bash
git clone https://github.com/sirajju/whatsapp-bot-with-ai.git
cd whatsapp-bot-with-ai
cp .env.example .env
# edit .env: set PAIR_PHONE and OWNERS

make build-linux
./bin/wabot-linux-amd64                 # first run prints a pairing code
```

In WhatsApp on your phone: **Settings → Linked devices → Link with phone
number** and enter the code shown in the terminal.

### systemd unit (for `!restart` to actually come back)

```ini
# /etc/systemd/system/wabot.service
[Unit]
Description=wabot
After=network-online.target

[Service]
User=sirajju
WorkingDirectory=/home/sirajju/whatsapp-bot-with-ai
ExecStart=/home/sirajju/whatsapp-bot-with-ai/bin/wabot
Restart=on-failure
RestartSec=5
EnvironmentFile=/home/sirajju/whatsapp-bot-with-ai/.env

[Install]
WantedBy=multi-user.target
```

## Commands (26)

### Core

| cmd         | what it does                                           |
|-------------|--------------------------------------------------------|
| `!menu`     | list every command                                     |
| `!ping`     | liveness + uptime                                      |

### AI

| cmd                 | what it does                                          |
|---------------------|-------------------------------------------------------|
| `!ai <prompt>`      | ask the configured LLM (remembers last 10 turns)      |
| `!clearai`          | wipe AI chat memory for this chat                     |
| `!persona [name]`   | switch persona per chat (or show current + list)      |

Personas: `default`, `coder`, `teacher`, `stoic`, `pirate`, `shaka`, `gordon`,
`therapist`, `detective`, `news`, `yoda`.

AI backend priority (whichever env var is set first wins):
`OLLAMA_URL` → `GROQ_API_KEY` → `ANTHROPIC_API_KEY` → `OPENAI_API_KEY` → `GEMINI_API_KEY`.
Groq has a free tier; Ollama is 100% local.

### Stickers

| cmd         | what it does                                            |
|-------------|---------------------------------------------------------|
| `!sticker`  | reply to image/video → WebP sticker (`!s` alias)        |
| `!toimg`    | reply to a sticker → JPEG image                         |
| `!stext`    | render text as a sticker                                |

### Downloader

| cmd              | what it does                                       |
|------------------|----------------------------------------------------|
| `!dl <url>`      | YouTube, TikTok, Reels, Twitter/X, FB, etc (yt-dlp) |

### Fun

| cmd                       | what it does                               |
|---------------------------|--------------------------------------------|
| `!tr <lang> <text>`       | translate via Lingva (no key)              |
| `!weather <city>`         | Open-Meteo (no key)                        |
| `!8ball <question>`       | magic 8-ball                               |
| `!roll NdM`               | dice, e.g. `!roll 2d20`                    |
| `!calc <expr>`            | arithmetic with + - * / % ^ and parens     |
| `!quote`                  | random quote                               |
| `!lyrics <artist> - <title>` | fetch lyrics via lyrics.ovh             |

### Group moderation (owner-only)

| cmd            | what it does                                            |
|----------------|---------------------------------------------------------|
| `!kick`        | remove quoted user (or by phone arg)                    |
| `!add <phone>` | invite a phone number                                   |
| `!promote`     | promote to admin                                        |
| `!demote`      | demote admin                                            |
| `!mute`        | lock group to admins-only                               |
| `!unmute`      | unlock group                                            |
| `!warn [why]`  | warn user (auto-kick at 3)                              |
| `!warns`       | show warn count for a user                              |
| `!unwarn`      | clear warns                                             |
| `!antilink on/off` | toggle auto-delete of links + warn                 |
| `!welcome <msg>` | set welcome template (`{user}`, `{group}`)            |
| `!goodbye <msg>` | set goodbye template                                  |

### Admin (owner-only)

| cmd         | what it does                                             |
|-------------|----------------------------------------------------------|
| `!broadcast <msg>` | send to every joined group (0.5s spacing)         |
| `!block`    | block a user from the bot                                |
| `!unblock`  | unblock                                                  |
| `!restart`  | exit (expects systemd / supervisor to respawn)           |

### Passive (always on)

- **Blocklist** — messages from blocked users are silently dropped before dispatch.
- **Antilink** — in groups where it's enabled, link-bearing messages are
  revoked (deleted) and the sender is auto-warned (→ kicked at 3).
- **Welcome / goodbye** — fires on join/leave in groups that have templates set.

## Project layout

```
cmd/wabot/              main entry
internal/config/        env loading
internal/auth/          whatsmeow session + pairing/QR login
internal/router/        command dispatcher + passive hook pipeline
internal/commands/      every command
internal/ai/            provider interface + 5 backends + personas
internal/media/         sticker encoding via ffmpeg
internal/store/         sqlite for group settings, warns, blocklist,
                        chat history, personas
```

## License

MIT. See `LICENSE`.
