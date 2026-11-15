#!/usr/bin/env bash
# wabot — manage the WhatsApp bot (whatsmeow) service.
set -euo pipefail

PROJECT_DIR="${WABOT_PROJECT_DIR:-$HOME/whatsapp-bot-with-ai}"
BIN="$PROJECT_DIR/bin/wabot"
LOG="$PROJECT_DIR/wabot.log"
UNIT="wabot.service"
SYSCTL="systemctl --user"

usage() {
	cat <<EOU
wabot — manage the WhatsApp bot (whatsmeow)

  wabot status             service status
  wabot start | stop | restart
  wabot reload             reload user systemd units
  wabot enable | disable   toggle auto-start on login

  wabot logs [-f] [-n N]   tail application log (default -n 50)
  wabot journal [-f]       tail systemd journal for the unit
  wabot tail               alias for 'logs -f'

  wabot build              rebuild the binary (go build)
  wabot deploy             build + restart
  wabot run                foreground run (stops service first — for debugging)

  wabot ps                 pid, cpu/mem, open sockets
  wabot env                show .env keys (not values)
  wabot path               show project paths

Env:
  WABOT_PROJECT_DIR        override project dir (default: \$HOME/whatsapp-bot-with-ai)
EOU
}

strip_ansi() { sed 's/\x1b\[[0-9;]*m//g'; }

cmd="${1:-help}"; shift || true

case "$cmd" in
	status)   exec $SYSCTL status  "$UNIT" ;;
	start)    exec $SYSCTL start   "$UNIT" ;;
	stop)     exec $SYSCTL stop    "$UNIT" ;;
	restart)  exec $SYSCTL restart "$UNIT" ;;
	reload)   exec $SYSCTL daemon-reload ;;
	enable)   exec $SYSCTL enable  "$UNIT" ;;
	disable)  exec $SYSCTL disable "$UNIT" ;;

	logs)
		n=50; follow=0
		while [[ $# -gt 0 ]]; do
			case "$1" in
				-f|--follow) follow=1; shift ;;
				-n) n="$2"; shift 2 ;;
				-n*) n="${1#-n}"; shift ;;
				*) echo "unknown arg: $1" >&2; exit 2 ;;
			esac
		done
		if (( follow )); then
			tail -n "$n" -f "$LOG" | strip_ansi
		else
			tail -n "$n" "$LOG" | strip_ansi
		fi
		;;
	tail) exec "$0" logs -f ;;

	journal)
		if [[ "${1:-}" == "-f" ]]; then exec journalctl --user -u "$UNIT" -f; fi
		exec journalctl --user -u "$UNIT" -n 50 --no-pager
		;;

	build)
		cd "$PROJECT_DIR"
		echo "building wabot…"
		go build -o "$BIN" ./cmd/wabot
		echo "built: $(ls -lh "$BIN" | awk '{print $5}')  $BIN"
		;;

	deploy)
		cd "$PROJECT_DIR"
		echo "building wabot…"
		go build -o "$BIN" ./cmd/wabot
		echo "restarting service…"
		$SYSCTL restart "$UNIT"
		sleep 2
		$SYSCTL status "$UNIT" --no-pager -l | head -10
		;;

	run)
		echo "stopping service for foreground run…"
		$SYSCTL stop "$UNIT" || true
		cd "$PROJECT_DIR"
		set -a; [ -f .env ] && . ./.env; set +a
		exec "$BIN"
		;;

	ps)
		pid=$($SYSCTL show -p MainPID --value "$UNIT")
		if [[ -z "$pid" || "$pid" == "0" ]]; then echo "not running"; exit 1; fi
		ps -o pid,etime,%cpu,%mem,rss,state,cmd -p "$pid"
		echo
		ss -tnp 2>/dev/null | awk -v p="pid=$pid," 'NR==1 || $0 ~ p'
		;;

	env)
		grep -oE '^[A-Z_]+' "$PROJECT_DIR/.env" | sort -u
		;;

	path)
		printf "project: %s\nbinary:  %s\nlog:     %s\nunit:    %s (user)\n" \
			"$PROJECT_DIR" "$BIN" "$LOG" "$UNIT"
		;;

	""|help|-h|--help) usage ;;
	*) echo "unknown command: $cmd" >&2; usage; exit 2 ;;
esac
