package commands

import (
	"context"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// sendText is a tiny helper: send a plain text reply to chat.
func sendText(ctx context.Context, cli *whatsmeow.Client, chat types.JID, text string) error {
	_, err := cli.SendMessage(ctx, chat, &waE2E.Message{
		Conversation: proto.String(text),
	})
	return err
}
