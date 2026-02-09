package notifications

import (
	"context"
	"testing"
)

func TestRestoreNotifier_NilServices(t *testing.T) {
	// Should not panic with nil services
	n := NewRestoreNotifier(nil, nil)

	ctx := context.Background()
	if err := n.SendRestoreTapeChangeRequired(ctx, "TAPE-001", "TAPE-002"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := n.SendRestoreWrongTape(ctx, "TAPE-001", "TAPE-002"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRestoreNotifier_DisabledServices(t *testing.T) {
	// Disabled services should not error
	telegram := NewTelegramService(TelegramConfig{Enabled: false})
	email := NewEmailService(EmailConfig{Enabled: false})

	n := NewRestoreNotifier(telegram, email)

	ctx := context.Background()
	if err := n.SendRestoreTapeChangeRequired(ctx, "TAPE-001", "TAPE-002"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := n.SendRestoreWrongTape(ctx, "TAPE-001", "TAPE-002"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
