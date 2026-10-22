// SPDX-License-Identifier: AGPL-3.0-only
package email

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"chastedesk/internal/db"
)

func DispatchOnce(ctx context.Context, repo *db.Repo, sender *Sender, logger *slog.Logger) (int, int, error) {
	if sender == nil || !sender.Enabled() {
		return 0, 0, nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	rows, err := repo.ClaimEmailOutboxAll(ctx, 20)
	if err != nil {
		return 0, 0, fmt.Errorf("email: claim outbox: %w", err)
	}
	sent, failed := 0, 0
	for _, row := range rows {
		messageID := row.EmailMessageID
		if messageID == "" {
			messageID = sender.MessageID()
			if err := repo.SetEmailOutboxMessageID(db.WithOrg(ctx, row.OrgID), row.ID, messageID); err != nil {
				logger.Warn("email outbox message id update failed", "outbox_id", row.ID, "error", err)
				continue
			}
		}
		references := row.References
		if references == "" && row.InReplyTo != "" {
			references = row.InReplyTo
		}
		sendCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		err := sender.Send(sendCtx, row.ToEmail, row.Subject, row.TextBody, row.HTMLBody, messageID, row.InReplyTo, references)
		cancel()
		if err != nil {
			failed++
			backoff := time.Duration(row.Attempts) * time.Minute
			if backoff < time.Minute {
				backoff = time.Minute
			}
			if backoff > 30*time.Minute {
				backoff = 30 * time.Minute
			}
			if markErr := repo.MarkEmailFailed(db.WithOrg(ctx, row.OrgID), row.ID, err.Error(), backoff); markErr != nil {
				logger.Warn("email outbox failure update failed", "outbox_id", row.ID, "error", markErr)
			}
			logger.Warn("email delivery failed", "outbox_id", row.ID, "attempt", row.Attempts, "error", err)
			continue
		}
		if err := repo.MarkEmailSent(db.WithOrg(ctx, row.OrgID), row.ID, messageID); err != nil {
			logger.Warn("email outbox sent update failed", "outbox_id", row.ID, "error", err)
			continue
		}
		sent++
	}
	return sent, failed, nil
}

func StartDispatcher(ctx context.Context, repo *db.Repo, sender *Sender, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	if sender == nil || !sender.Enabled() {
		logger.Info("email delivery disabled; SMTP is not configured")
		return
	}
	run := func() {
		runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		sent, failed, err := DispatchOnce(runCtx, repo, sender, logger)
		if err != nil {
			logger.Warn("email dispatcher failed", "error", err)
			return
		}
		if sent > 0 || failed > 0 {
			logger.Info("email dispatcher completed", "sent", sent, "failed", failed)
		}
	}
	run()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
