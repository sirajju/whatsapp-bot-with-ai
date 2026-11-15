package commands

import (
	"context"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func sendText(ctx context.Context, cli *whatsmeow.Client, chat types.JID, text string) error {
	_, err := cli.SendMessage(ctx, chat, &waE2E.Message{
		Conversation: proto.String(text),
	})
	return err
}

func stringPtr(s string) *string { return &s }
func uint64Ptr(u uint64) *uint64 { return &u }
