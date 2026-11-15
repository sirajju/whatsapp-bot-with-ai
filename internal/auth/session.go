// Package auth wires whatsmeow's SQLite session store and handles login.
package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mdp/qrterminal/v3"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"

	_ "modernc.org/sqlite"
)

// NewClient builds a whatsmeow client backed by a SQLite session at sessionDir/store.db.
// A device is picked (first if the store has one, otherwise a fresh one).
func NewClient(ctx context.Context, sessionDir string, log zerolog.Logger) (*whatsmeow.Client, error) {
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir session: %w", err)
	}

	dbPath := filepath.Join(sessionDir, "store.db")
	// busy_timeout=5s lets whatsmeow's parallel writers wait instead of returning SQLITE_BUSY
	// when the prekey/identity store sees a burst (history sync, multi-device chats).
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)

	container, err := sqlstore.New(ctx, "sqlite", dsn, waLog.Noop)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: %w", err)
	}

	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}

	cli := whatsmeow.NewClient(device, zerologAdapter{log: log.With().Str("component", "whatsmeow").Logger()})
	return cli, nil
}

// LoginPairing registers a pairing code login flow. Phone is digits only, country code included (e.g. 919xxxxxxxxx).
// It prints the pairing code to stdout and expects the user to enter it in their WhatsApp mobile app.
func LoginPairing(ctx context.Context, cli *whatsmeow.Client, phone string, log zerolog.Logger) error {
	if cli.Store.ID != nil {
		return cli.Connect()
	}
	if phone == "" {
		return errors.New("pair phone required")
	}

	qrChan, _ := cli.GetQRChannel(ctx)
	if err := cli.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	// Drain QR events in background (whatsmeow produces them even in pair-code mode).
	go func() {
		for range qrChan {
		}
	}()

	code, err := cli.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		return fmt.Errorf("pair code: %w", err)
	}
	log.Info().Str("code", code).Msg("Enter this pairing code in WhatsApp > Linked Devices > Link with phone number")
	fmt.Printf("\n\n  PAIRING CODE:  %s\n\n", code)
	return nil
}

// LoginQR prints a QR to the terminal and waits for scan. Used when LOGIN_MODE=qr.
func LoginQR(ctx context.Context, cli *whatsmeow.Client, log zerolog.Logger) error {
	if cli.Store.ID != nil {
		return cli.Connect()
	}
	qrChan, _ := cli.GetQRChannel(ctx)
	if err := cli.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	go func() {
		for evt := range qrChan {
			switch evt.Event {
			case "code":
				fmt.Println("\nScan this QR with WhatsApp > Linked Devices:")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			case "success":
				log.Info().Msg("Linked")
			case "timeout":
				log.Warn().Msg("QR timeout")
			}
		}
	}()
	return nil
}

// zerologAdapter bridges zerolog into whatsmeow's waLog.Logger interface.
type zerologAdapter struct{ log zerolog.Logger }

func (z zerologAdapter) Warnf(msg string, args ...any)  { z.log.Warn().Msgf(msg, args...) }
func (z zerologAdapter) Errorf(msg string, args ...any) { z.log.Error().Msgf(msg, args...) }
func (z zerologAdapter) Infof(msg string, args ...any)  { z.log.Info().Msgf(msg, args...) }
func (z zerologAdapter) Debugf(msg string, args ...any) { z.log.Debug().Msgf(msg, args...) }
func (z zerologAdapter) Sub(mod string) waLog.Logger {
	return zerologAdapter{log: z.log.With().Str("mod", mod).Logger()}
}
