# whatsapp-bot-with-ai

A lightweight, self-hosted WhatsApp bot written in Go on top of
[`whatsmeow`](https://github.com/tulir/whatsmeow). Feature set modelled on
[FantoX/Atlas-MD](https://github.com/FantoX/Atlas-MD) (reference only — no code
reuse). Designed for low-RAM hardware: a single static binary, no browser, no
Node.js runtime.

## Why this stack

| Thing          | Choice                                  | Why                                   |
|----------------|-----------------------------------------|---------------------------------------|
| Language       | Go 1.23                                 | Static binary, low memory             |
| WhatsApp lib   | `go.mau.fi/whatsmeow`                   | WebSocket client, no headless Chrome  |
| Session store  | `modernc.org/sqlite` (pure Go)          | No cgo, cross-compiles trivially      |
| Logger         | `rs/zerolog`                            | Structured, fast                      |
| AI             | Interface + 5 backends (all optional)   | Bot works without any AI credit       |

Idle footprint: ~25 MB RSS. With an active session: ~50–80 MB.

## Hardware requirements

- Linux x86_64 (also builds on arm64)
- **512 MB RAM** is enough; 1 GB is comfortable
- 500 MB disk (binary + session DB + cache)
- `ffmpeg` on PATH if you want the `!sticker` command

## Install & run

```bash
git clone https://github.com/sirajju/whatsapp-bot-with-ai.git
cd whatsapp-bot-with-ai
cp .env.example .env
# edit .env: set PAIR_PHONE and (optionally) one AI_*_API_KEY

make build-linux                        # produces bin/wabot-linux-amd64
./bin/wabot-linux-amd64                 # first run prints a pairing code
```

In WhatsApp on your phone: **Settings → Linked devices → Link with phone number**
and enter the pairing code printed in the terminal.

### systemd unit (optional)

```ini
# /etc/systemd/system/wabot.service
[Unit]
Description=wabot
After=network-online.target

[Service]
User=sirajju
WorkingDirectory=/home/sirajju/whatsapp-bot-with-ai
ExecStart=/home/sirajju/whatsapp-bot-with-ai/bin/wabot-linux-amd64
Restart=on-failure
RestartSec=5
EnvironmentFile=/home/sirajju/whatsapp-bot-with-ai/.env

[Install]
WantedBy=multi-user.target
```

## Commands

- `!menu` — list every command
- `!ping` — liveness + uptime
- `!ai <prompt>` — call the configured LLM
- `!sticker` — reply to an image/video to turn it into a WhatsApp sticker
- `!kick` — owner-only: remove the quoted user from a group

## AI provider priority

Whichever env var is set first (highest priority wins):
`OLLAMA_URL` → `GROQ_API_KEY` → `ANTHROPIC_API_KEY` → `OPENAI_API_KEY` → `GEMINI_API_KEY`.
Groq has a free tier; Ollama is 100% local.

## Project layout

```
cmd/wabot/              main entry
internal/config/        env loading
internal/auth/          whatsmeow session + pairing/QR login
internal/router/        command dispatcher
internal/commands/      ping, menu, ai, sticker, kick
internal/ai/            provider interface + 5 backends
internal/media/         sticker encoding via ffmpeg
```

## License

MIT. See `LICENSE`.
