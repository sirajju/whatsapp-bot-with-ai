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

make install              # builds, installs the user systemd unit + `wabot` CLI, starts the service
```

On first run, link the device: **Settings → Linked devices → Link with phone
number** in WhatsApp, then enter the pairing code shown in `wabot logs -f`.

### The `wabot` CLI

The installer puts a management wrapper on your PATH:

```
wabot status             service status
wabot start | stop | restart
wabot logs [-f] [-n N]   tail application log
wabot journal [-f]       tail systemd journal
wabot build              rebuild the binary
wabot deploy             build + restart
wabot run                foreground run (stops service first — debug)
wabot ps                 pid, cpu/mem, open sockets
wabot env                show .env keys (not values)
wabot path               show project paths
```

### Manual systemd setup (if you skip `make install`)

The installer writes a **user** systemd unit to `~/.config/systemd/user/wabot.service`
rendered from `scripts/wabot.service`. If you want to roll your own, copy that
template and replace `{{PROJECT_DIR}}` with your clone path.

```bash
mkdir -p ~/.config/systemd/user
sed "s|{{PROJECT_DIR}}|$PWD|g" scripts/wabot.service > ~/.config/systemd/user/wabot.service
systemctl --user daemon-reload
systemctl --user enable --now wabot.service
sudo loginctl enable-linger $USER   # survive logout
```

## Commands

### Core

| cmd         | what it does                                           |
|-------------|--------------------------------------------------------|
| `!menu`     | list every command                                     |
| `!ping`     | liveness + uptime                                      |

### AI

| cmd                 | what it does                                          |
|---------------------|-------------------------------------------------------|
| `!ai`               | turn on AI session mode (every message goes to the LLM) |
| `!ai <prompt>`      | single-turn ask (remembers last 10 turns of history)  |
| `!endai`            | leave AI session mode                                 |
| `!clearai`          | wipe AI chat memory for this chat                     |
| `!persona [name]`   | switch persona per chat (or show current + list)      |

**Default behavior:** on the first message from a new DM the bot sends a short
welcome and enables AI mode automatically — the user can `!endai` to turn it
off. If the AI provider returns an error, AI mode auto-disables for that chat
so the user isn't stuck in a broken loop.

**WhatsApp-aware output:** the bot's system prompt tells every persona it's
inside WhatsApp, and replies are post-processed so stray Markdown (`**bold**`,
headings, `[link](url)`) and HTML entities (`&amp;`, `<br>`) become native
WhatsApp syntax.

Personas: `default`, `coder`, `teacher`, `stoic`, `pirate`, `shaka`, `gordon`,
`therapist`, `detective`, `news`, `yoda`.

AI backend priority (whichever env var is set first wins):
`OPENCODE_CMD` → `OLLAMA_URL` → `GROQ_API_KEY` → `NVIDIA_API_KEY` → `OPENROUTER_API_KEY` →
`ANTHROPIC_API_KEY` → `OPENAI_API_KEY` → `GEMINI_API_KEY`. Set `AI_PREFERRED` to pin one
explicitly. Groq has a generous free tier; Ollama is 100% local.

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

| cmd                         | what it does                                     |
|-----------------------------|--------------------------------------------------|
| `!send <phone> <msg>`       | DM anyone from the bot number (phone in E.164 digits) |
| `!broadcast <msg>`          | send to every joined group (0.5s spacing)        |
| `!block`                    | block a user from the bot                        |
| `!unblock`                  | unblock                                          |
| `!restart`                  | exit (expects systemd / supervisor to respawn)   |
| `!sh <cmd>`                 | run a bash command on the host                   |
| `!status`                   | RAM / disk / uptime snapshot                     |

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
