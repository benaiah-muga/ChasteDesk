// SPDX-License-Identifier: AGPL-3.0-only
package views

func otherScope(scope string) string {
	if scope == "internal" {
		return "public"
	}
	return "internal"
}
