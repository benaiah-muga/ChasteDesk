// SPDX-License-Identifier: AGPL-3.0-only
package email

import (
	"fmt"
	"html"
	"strings"

	"chastedesk/internal/db"
)

func EnqueueInvitation(ctx db.RepoCtx, repo *db.Repo, orgName, to, link string) (string, error) {
	subject := "You have been invited to " + orgName + " on ChasteDesk"
	text := fmt.Sprintf("You have been invited to join %s on ChasteDesk.\n\nAccept the invitation:\n%s\n\nThis single-use link expires in seven days. If you were not expecting this invitation, you can ignore it.\n", orgName, link)
	body := "<!doctype html><html><body style=\"font-family: system-ui, sans-serif; line-height: 1.6; color: #211b16; max-width: 560px; margin: 0 auto; padding: 24px;\">" +
		"<h1 style=\"font-size: 22px; margin: 0 0 12px;\">Join " + html.EscapeString(orgName) + " on ChasteDesk</h1>" +
		"<p>You have been invited to the support desk.</p>" +
		"<p style=\"margin: 24px 0;\"><a href=\"" + html.EscapeString(link) + "\" style=\"background: #3b2a20; color: #f7f1e7; padding: 10px 16px; border-radius: 6px; text-decoration: none; font-weight: 600;\">Accept invitation</a></p>" +
		"<p style=\"font-size: 12px; color: #6f6258;\">This single-use link expires in seven days. If you were not expecting this invitation, you can ignore it.</p>" +
		"<p style=\"font-size: 11px; color: #8b7c70; word-break: break-all;\">" + html.EscapeString(link) + "</p>" +
		"</body></html>"
	return repo.EnqueueEmail(ctx, "invitation", strings.ToLower(strings.TrimSpace(to)), subject, text, body)
}
