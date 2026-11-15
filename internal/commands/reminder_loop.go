package commands

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/store"
)

// StartReminderLoop launches a goroutine that polls the reminders table every 30s
// and sends due reminders to their chats. Cancel by cancelling ctx.
func StartReminderLoop(ctx context.Context, cli *whatsmeow.Client, st *store.Store, log zerolog.Logger) {
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				due, err := st.DueReminders(ctx, now.Unix())
				if err != nil {
					log.Debug().Err(err).Msg("reminder poll")
					continue
				}
				for _, r := range due {
					// Reuse the Env-less helper below to send text
					jid, err := router.ParseJIDSafe(r.ChatJID)
					if err != nil {
						_ = st.MarkFired(ctx, r.ID)
						continue
					}
					_ = sendText(ctx, cli, jid, "⏰ reminder: "+r.Text)
					_ = st.MarkFired(ctx, r.ID)
				}
			}
		}
	}()
}
