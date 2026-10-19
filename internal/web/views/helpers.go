// SPDX-License-Identifier: AGPL-3.0-only
package views

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

func otherScope(scope string) string {
	if scope == "internal" {
		return "public"
	}
	return "internal"
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

func currentAttr(active, item string) string {
	if active == item {
		return "page"
	}
	return ""
}

func countText(n int) string {
	if n > 99 {
		return "99+"
	}
	return fmt.Sprintf("%d", n)
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func urlQueryEscape(value string) string {
	return url.QueryEscape(value)
}

func itoa64(n int64) string {
	return fmt.Sprintf("%d", n)
}

func RelativeTime(t time.Time) string {
	return relativeTime(t)
}

func ChannelLabel(channel string) string {
	return channelLabel(channel)
}

func InitialOf(name string) string {
	return initial(name)
}

func StringOr(value, fallback string) string {
	return stringOr(value, fallback)
}

func CountLabel(n int, singular, plural string) string {
	return countLabel(n, singular, plural)
}

func ProviderLabel(id string) string {
	return providerLabel(id)
}

func ShortDate(value string) string {
	return shortDate(value)
}
