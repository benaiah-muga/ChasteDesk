// SPDX-License-Identifier: AGPL-3.0-only
package views

import (
	"fmt"
	"strings"
)

func otherScope(scope string) string {
	if scope == "internal" {
		return "public"
	}
	return "internal"
}

func badgeClass(status string) string {
	switch status {
	case "resolved", "closed":
		return "success"
	case "awaiting_human":
		return "warn"
	}
	return ""
}

func joinStrings(values []string, sep string) string {
	return strings.Join(values, sep)
}

func openConversationLabel(n int) string {
	if n == 1 {
		return "1 other open conversation"
	}
	return fmt.Sprintf("%d other open conversations", n)
}

func pendingActionCount(actions []InboxActionView) int {
	count := 0
	for _, action := range actions {
		if action.Pending {
			count++
		}
	}
	return count
}
