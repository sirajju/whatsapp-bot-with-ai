#!/usr/bin/env bash
# wabot installer — builds, installs the user systemd unit, installs the
# `wabot` CLI helper, and starts the service.
#
# Usage:  ./scripts/install.sh    (from repo root)
#         or:  make install
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
UNIT_DIR="$HOME/.config/systemd/user"
UNIT_SRC="$PROJECT_DIR/scripts/wabot.service"
CLI_SRC="$PROJECT_DIR/scripts/wabot-cli.sh"
CLI_DST="$HOME/.local/bin/wabot"

say() { printf '→ %s\n' "$*"; }
ok()  { printf '✓ %s\n' "$*"; }
die() { printf '✗ %s\n' "$*" >&2; exit 1; }

say "wabot installer"
printf '  project: %s\n' "$PROJECT_DIR"

# 1. Dependencies -----------------------------------------------------------
command -v go >/dev/null 2>&1 || die "Go not found. Install Go 1.23+ from https://go.dev/dl/"
printf '  go:      %s\n' "$(go version | awk '{print $3,$4}')"
command -v systemctl >/dev/null 2>&1 || die "systemctl not found (this installer uses user systemd units)"

# 2. .env -------------------------------------------------------------------
if [ ! -f "$PROJECT_DIR/.env" ]; then
	if [ -f "$PROJECT_DIR/.env.example" ]; then
		cp "$PROJECT_DIR/.env.example" "$PROJECT_DIR/.env"
		ok "created .env from template — edit it before first run"
	else
		die "no .env or .env.example in $PROJECT_DIR"
	fi
fi

# 3. Build ------------------------------------------------------------------
say "building binary"
cd "$PROJECT_DIR"
make build

# 4. Install systemd user unit ---------------------------------------------
say "installing systemd user unit"
mkdir -p "$UNIT_DIR"
sed "s|{{PROJECT_DIR}}|$PROJECT_DIR|g" "$UNIT_SRC" > "$UNIT_DIR/wabot.service"
systemctl --user daemon-reload

# 5. Linger so the service survives a logout --------------------------------
if command -v loginctl >/dev/null 2>&1; then
	if ! loginctl show-user "$USER" --property=Linger 2>/dev/null | grep -q yes; then
		say "enabling user linger (sudo required)"
		if sudo -n true 2>/dev/null; then
			sudo loginctl enable-linger "$USER" && ok "linger enabled"
		else
			printf '  (skipped — run: sudo loginctl enable-linger %s)\n' "$USER"
		fi
	fi
fi

# 6. Install the `wabot` CLI helper on PATH --------------------------------
if [ -f "$CLI_SRC" ]; then
	mkdir -p "$HOME/.local/bin"
	install -m 755 "$CLI_SRC" "$CLI_DST"
	ok "installed CLI: $CLI_DST"
	# Make sure ~/.local/bin is on PATH for future shells
	if ! grep -qF 'HOME/.local/bin' "$HOME/.bashrc" 2>/dev/null; then
		cat >> "$HOME/.bashrc" <<'BRC'

# added by wabot installer
if [ -d "$HOME/.local/bin" ] && [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
	export PATH="$HOME/.local/bin:$PATH"
fi
BRC
	fi
fi

# 7. Enable + start ---------------------------------------------------------
systemctl --user enable wabot.service
systemctl --user restart wabot.service
sleep 2
systemctl --user --no-pager status wabot.service | head -8 || true

echo
ok "installed. CLI: 'wabot status', 'wabot logs -f', 'wabot deploy'"
