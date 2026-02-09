package notifications

import (
	"context"
	"fmt"
)

// RestoreNotifier sends restore-specific notifications via all configured
// channels (email and/or telegram).
type RestoreNotifier struct {
	telegram *TelegramService
	email    *EmailService
}

// NewRestoreNotifier creates a new RestoreNotifier.
// Either service may be nil if not configured.
func NewRestoreNotifier(telegram *TelegramService, email *EmailService) *RestoreNotifier {
	return &RestoreNotifier{telegram: telegram, email: email}
}

// SendRestoreTapeChangeRequired notifies the user that a different tape
// is needed for the restore to continue.
func (n *RestoreNotifier) SendRestoreTapeChangeRequired(ctx context.Context, expectedLabel string, actualLabel string) error {
	if n.telegram != nil && n.telegram.IsEnabled() {
		_ = n.telegram.NotifyTapeChangeRequired(ctx, "Restore", actualLabel, fmt.Sprintf("Please insert tape %s to continue the restore", expectedLabel), expectedLabel)
	}
	if n.email != nil && n.email.IsEnabled() {
		_ = n.email.NotifyTapeChangeRequired(ctx, "Restore", actualLabel, fmt.Sprintf("Please insert tape %s to continue the restore", expectedLabel), expectedLabel)
	}
	return nil
}

// SendRestoreWrongTape notifies the user that the wrong tape is loaded.
func (n *RestoreNotifier) SendRestoreWrongTape(ctx context.Context, expectedLabel string, actualLabel string) error {
	if n.telegram != nil && n.telegram.IsEnabled() {
		_ = n.telegram.NotifyWrongTapeInserted(ctx, expectedLabel, actualLabel)
	}
	// Email doesn't have a specific wrong-tape method; reuse tape change.
	if n.email != nil && n.email.IsEnabled() {
		_ = n.email.NotifyTapeChangeRequired(ctx, "Restore", actualLabel, fmt.Sprintf("Wrong tape loaded — please insert tape %s", expectedLabel), expectedLabel)
	}
	return nil
}
