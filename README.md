# whatsapp-bot-with-ai

A lightweight, self-hosted WhatsApp bot written in Go on top of [`whatsmeow`](https://github.com/tulir/whatsmeow). Designed for low-RAM homelab hardware: a single static binary, no browser, and no Node.js runtime.

In addition to rich bot capabilities (AI, voice notes, stickers, downloaders, moderation), it integrates deeply with your home: **LAN presence tracking**, **wall-phone CCTV snapshot & live streaming**, **two-way intercom**, **household doorbell/ping alerts**, **security audits**, and **system health monitoring**.

## Why this stack

| Component      | Choice                                  | Why                                   |
|----------------|-----------------------------------------|---------------------------------------|
| Language       | Go 1.23+                                | Static binary, minimal memory         |
| WhatsApp lib   | `go.mau.fi/whatsmeow`                   | Native WebSocket client, no browser   |
| Storage        | `modernc.org/sqlite` (pure Go)          | No cgo, cross-compiles trivially      |
| Logger         | `rs/zerolog`                            | Structured, high performance          |
| AI             | Interface + 8 backends (all optional)   | Bot works without any AI key          |
| Hardware       | Raspberry Pi / Mini PC / Homelab Server | Stripped binary: ~18 MB, Idle RSS: ~25 MB |

- Stripped binary: **18 MB**
- Idle memory: ~25 MB RSS
- Active memory: ~50–80 MB RSS

## Hardware & System requirements

- Linux x86_64 or arm64
- **512 MB RAM** is enough; 1 GB is comfortable
- 500 MB disk
- `ffmpeg` on PATH for sticker and audio processing
- `yt-dlp` on PATH for the `!dl` video downloader

## Install & Run

```bash
git clone https://github.com/sirajju/whatsapp-bot-with-ai.git
cd whatsapp-bot-with-ai
cp .env.example .env
# edit .env: set PAIR_PHONE and OWNERS

make install              # builds, installs user systemd unit + `wabot` CLI, starts the service
```

On first run, link the device: in WhatsApp go to **Settings → Linked devices → Link with phone number**, then enter the pairing code displayed in `wabot logs -f`.

### The `wabot` CLI

The installer places a management wrapper on your PATH:

```bash
wabot status             # service status
wabot start | stop | restart
wabot logs [-f] [-n N]   # tail application log
wabot journal [-f]       # tail systemd journal
wabot build              # rebuild the binary
wabot deploy             # build + restart
wabot run                # foreground debug run (stops service first)
wabot ps                 # pid, cpu/mem, open sockets
wabot env                # show .env keys (not values)
wabot path               # show project paths
```

### Manual systemd setup (if skipping `make install`)

The installer writes a **user** systemd unit to `~/.config/systemd/user/wabot.service` rendered from `scripts/wabot.service`:

```bash
mkdir -p ~/.config/systemd/user
sed "s|{{PROJECT_DIR}}|$PWD|g" scripts/wabot.service > ~/.config/systemd/user/wabot.service
systemctl --user daemon-reload
systemctl --user enable --now wabot.service
sudo loginctl enable-linger $USER   # keep running after logout
```

---

## Commands

### Core & Admin (owner-only where noted)

| Command                     | Alias      | Access     | Description                                                     |
|-----------------------------|------------|------------|-----------------------------------------------------------------|
| `!menu`                     | `!help`    | All        | List available commands                                         |
| `!ping`                     |            | All        | Liveness check + uptime                                         |
| `!whoami`                   |            | All        | Show sender JID, name, and owner authorization status           |
| `!status`                   | `!sysinfo` | Owner      | Host snapshot: CPU load, RAM, disk usage, uptime                |
| `!sh <command>`             |            | Owner      | Execute a bash command on the host                              |
| `!restart`                  |            | Owner      | Gracefully exit (systemd respawns the service)                  |
| `!send <phone> <text>`      |            | Owner      | DM someone directly from the bot number (E.164 digits)          |
| `!broadcast <message>`      |            | Owner      | Send an announcement to every joined group (0.5s spacing)        |
| `!block` / `!unblock`       |            | Owner      | Block or unblock a user from interacting with the bot           |

### Smart Home, CCTV & Security

| Command                     | Alias      | Access     | Description                                                     |
|-----------------------------|------------|------------|-----------------------------------------------------------------|
| `!intruder`                 | `!check`, `!security` | Owner | **Security audit**: checks LAN presence + captures live snapshot |
| `!cctv`                     | `!cam`, `!snap`       | Owner | Snapshot from wall-phone camera (wakes IP Webcam via MQTT)     |
| `!cctv on [seconds]`        |            | Owner      | Starts live MJPEG camera stream with auto-off timer (max 10m)    |
| `!cctv off`                 |            | Owner      | Shuts off the live camera stream immediately                    |
| `!speedtest`                | `!speed`   | Owner      | Network latency, download & upload speed test via Cloudflare CDN|
| `!gemini [live/app/off]`    | `!assistant` | Owner    | Trigger official Gemini Voice Assistant / Live on the wall phone |

### LAN Presence Tracking

Integrates with [`wifi-presence`](https://github.com/sirajju/wifi-presence) to track devices connected to your home Wi-Fi:

| Command                         | Alias    | Access | Description                                                 |
|---------------------------------|----------|--------|-------------------------------------------------------------|
| `!home`                         | `!who`   | Owner  | List who/which devices are on the home network right now    |
| `!devices`                      |          | Owner  | Show complete registry of known LAN devices                 |
| `!label <mac> <name> [owner]`   |          | Owner  | Assign friendly name and optional owner tag to a MAC address|

- **Arrival/Departure Alerts**: When `PRESENCE_ALERTS=on`, the bot DMs the owner whenever someone arrives home, leaves, or a new unrecognized device joins the network.

### Wall-Phone Intercom & Household Automation

- **WhatsApp to Wall-Phone Intercom**: Send a voice note to the bot number from an owner phone — the bot downloads the audio, converts it, and publishes an MQTT event so the wall-phone speaker announces it immediately.
- **Wall-Phone Kiosk to WhatsApp (`/mic`)**: The embedded wall-phone kiosk serves a web interface with a hold-to-record microphone. Audio is sent to `POST /voice-from-home` and delivered directly to the owner's WhatsApp as an audio voice note.
- **Household "Ping me" Button**: When anyone presses the physical or Home Assistant "Ping me" button (published to `household/ping`), the bot automatically wakes the living room camera, captures a snapshot, and DMs the owner with photo and caption *"Someone at home is calling you."*
- **System Health Monitor**: Background loop checks CPU load, RAM, disk space, and thermal sensors. Sends an alert DM if limits are exceeded.
- **Startup Notification**: DMs `🟢 wabot up on <hostname>` to the owner upon establishing WhatsApp connection.

### AI & Assistant

| Command                 | Alias            | Description                                           |
|-------------------------|------------------|-------------------------------------------------------|
| `!ai`                   | `!chat`, `!gpt`  | Enter continuous AI chat session mode                 |
| `!ai <prompt>`          |                  | Single-turn question (remembers last 10 turns)        |
| `!endai`                |                  | Exit AI chat mode                                     |
| `!clearai`              |                  | Wipe AI conversation memory for this chat             |
| `!persona [name]`       |                  | Switch AI persona per chat (or show list)             |

- **WhatsApp-aware formatting**: Responses are cleaned and post-processed for native WhatsApp formatting (`*bold*`, `_italics_`, `~strike~`, monospace).
- **Auto-greeting**: Automatically greets first-time DM senders and enters AI mode.
- **Function Calling**: AI provider supports tool calling (LAN presence, status, etc.) via `aitools` registry.
- **Personas**: `default`, `coder`, `teacher`, `stoic`, `pirate`, `shaka`, `gordon`, `therapist`, `detective`, `news`, `yoda`.
- **Supported Providers**: Ollama (local), Groq, NVIDIA NIM, OpenRouter, Anthropic Claude, OpenAI, Google Gemini, and local Opencode CLI.

### Voice & Audio (ElevenLabs & Groq Whisper)

| Command                     | Alias          | Description                                                 |
|-----------------------------|----------------|-------------------------------------------------------------|
| `!tts <text>`               |                | Convert text into a WhatsApp voice note                     |
| `!ttsvoice [voice]`         |                | Pick voice for text-to-speech                               |
| `!voice <name> <text>`      |                | Speak text using a cloned voice                             |
| `!clonevoice <name>`        |                | (Owner) Clone a voice from quoted voice note                |
| `!voices`                   |                | List registered cloned voices                               |
| `!stt`                      | `!transcribe`  | Reply to a voice note → transcribe text (Groq Whisper)      |
| `!sts <name>`               |                | Reply to a voice note → re-voice using a cloned voice       |

### Productivity & Tools

| Command                           | Alias    | Description                                             |
|-----------------------------------|----------|---------------------------------------------------------|
| `!remind <duration> <msg>`        |          | Set reminder (e.g. `!remind 45m check oven`, `!remind 2d call dad`) |
| `!reminders`                      |          | List pending reminders                                  |
| `!unremind <id>`                  |          | Cancel reminder by ID                                   |
| `!todo [add/list/done/del]`       |          | Personal todo list in chat                              |
| `!expense`                        | `!exp`   | Shared expense tracker (add, balance, settle up)        |
| `!skill <name> [args]`            |          | Run an automation skill script from `./skills/`         |
| `!skills`                         |          | List available skills                                   |
| `!run <script>`                   |          | (Owner) Execute a script from `./scripts/`              |

### Media & Stickers

| Command         | Alias   | Description                                            |
|-----------------|---------|--------------------------------------------------------|
| `!sticker`      | `!s`    | Reply to image/video → animated or static WebP sticker |
| `!toimg`        |         | Reply to a sticker → JPEG image                        |
| `!stext <text>` |         | Render text as a sticker                              |
| `!dl <url>`     | `!download` | Download video from YouTube, TikTok, Reels, X, etc. |

### Utilities & Fun

| Command                       | Alias   | Description                                          |
|-------------------------------|---------|------------------------------------------------------|
| `!calc <expression>`          |         | Math calculator (`+`, `-`, `*`, `/`, `%`, `^`, parens)|
| `!tr <lang> <text>`           | `!translate` | Translate text via Lingva (no API key required) |
| `!weather <city>`             |         | Current weather and forecast via Open-Meteo          |
| `!wiki <query>`               |         | Wikipedia summary lookup                             |
| `!define <word>`              | `!def`  | Dictionary definition                                |
| `!urban <term>`               |         | Urban Dictionary lookup                              |
| `!joke`                       |         | Random joke                                          |
| `!quote`                      |         | Inspirational quote                                  |
| `!lyrics <artist> - <title>`  |         | Fetch song lyrics via lyrics.ovh                     |
| `!8ball <question>`           |         | Magic 8-ball answer                                  |
| `!roll [NdM]`                 |         | Roll dice (e.g. `!roll 2d20`)                         |
| `!short <url>`                |         | Shorten link via is.gd                               |
| `!qr <text>`                  |         | Generate QR code image                               |
| `!meme <template> / <t1> / <t2>` |      | Create custom meme image                             |

### Group Moderation (owner-only)

| Command            | Description                                              |
|--------------------|----------------------------------------------------------|
| `!kick`            | Remove quoted user (or by phone number argument)         |
| `!add <phone>`     | Add/invite a phone number to group                       |
| `!promote`         | Promote quoted user to group admin                       |
| `!demote`          | Demote admin to regular member                           |
| `!mute`            | Lock group to admins-only messages                       |
| `!unmute`          | Unlock group for all members                             |
| `!warn [reason]`   | Issue warning to user (auto-kick on 3rd warning)         |
| `!warns`           | View warning count for user                              |
| `!unwarn`          | Reset warnings for user                                  |
| `!antilink on/off` | Automatically delete link messages and warn senders      |
| `!welcome <msg>`   | Configure group welcome greeting (`{user}`, `{group}`)   |
| `!goodbye <msg>`   | Configure group departure message                        |

---

## Project Layout

```
cmd/wabot/              # Main entrypoint, dependency wiring, service loops
internal/
  ai/                   # AI provider interface, backends (Ollama, Groq, Claude, etc.), personas
  aitools/              # Tool registry for AI function calling
  auth/                 # whatsmeow session init, QR and pairing code login
  commands/             # Handlers for every !command
  config/               # Environment variable parsing and defaults
  household/            # MQTT household button / ping subscriber with snapshot
  intercom/             # Two-way wall-phone audio bridge (MQTT + HTTP /mic page)
  media/                # Ffmpeg image/video/sticker transcoding
  monitor/              # System health prober (RAM, disk, CPU load, temperature)
  router/               # Message dispatcher, command registry, passive hooks pipeline
  store/                # SQLite persistence (group settings, warns, blocklist, AI memory)
scripts/                # Service units, installer, and wabot CLI wrapper
skills/                 # Modular automation skills (e.g. expense tracker)
```

## License

MIT. See `LICENSE`.
