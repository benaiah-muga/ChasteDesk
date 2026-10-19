// SPDX-License-Identifier: AGPL-3.0-only
package ingest

import (
	"regexp"
	"strings"
)

var (
	commentRe    = regexp.MustCompile(`(?s)<!--.*?-->`)
	titleRe      = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	h1Re         = regexp.MustCompile(`(?i)<h1[^>]*>`)
	h2Re         = regexp.MustCompile(`(?i)<h2[^>]*>`)
	h3Re         = regexp.MustCompile(`(?i)<h3[^>]*>`)
	headingEndRe = regexp.MustCompile(`(?i)</h[1-6]>`)
	liRe         = regexp.MustCompile(`(?i)<li[^>]*>`)
	breakTagRe   = regexp.MustCompile(`(?i)</p>|<br\s*/?>|</div>|</tr>`)
	cellEndRe    = regexp.MustCompile(`(?i)</t[dh]>`)
	tagRe        = regexp.MustCompile(`<[^>]+>`)
	entityRe     = regexp.MustCompile(`&(amp|lt|gt|quot|#39|nbsp);`)
)

var entityMap = map[string]string{
	"&amp;":  "&",
	"&lt;":   "<",
	"&gt;":   ">",
	"&quot;": `"`,
	"&#39;":  "'",
	"&nbsp;": " ",
}

var strippedTags = []string{"script", "style", "noscript", "svg", "nav", "footer", "header"}

func decodeEntities(text string) string {
	return entityRe.ReplaceAllStringFunc(text, func(match string) string {
		if replacement, ok := entityMap[match]; ok {
			return replacement
		}
		return match
	})
}

func stripTagBlocks(html, tag string) string {
	re := regexp.MustCompile(`(?is)<` + tag + `\b[^>]*>.*?</` + tag + `>`)
	return re.ReplaceAllString(html, "")
}

func HtmlToMarkdown(html string) (markdown string, title string) {
	text := commentRe.ReplaceAllString(html, "")
	for _, tag := range strippedTags {
		text = stripTagBlocks(text, tag)
	}
	if match := titleRe.FindStringSubmatch(text); match != nil {
		title = strings.TrimSpace(decodeEntities(match[1]))
	}
	if title == "" {
		title = "Untitled"
	}
	text = h1Re.ReplaceAllString(text, "\n# ")
	text = h2Re.ReplaceAllString(text, "\n## ")
	text = h3Re.ReplaceAllString(text, "\n### ")
	text = headingEndRe.ReplaceAllString(text, "\n")
	text = liRe.ReplaceAllString(text, "\n- ")
	text = breakTagRe.ReplaceAllString(text, "\n")
	text = cellEndRe.ReplaceAllString(text, " | ")
	text = tagRe.ReplaceAllString(text, "")
	text = decodeEntities(text)

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	kept := make([]string, 0, len(lines))
	for i, line := range lines {
		if line != "" || (i > 0 && lines[i-1] != "") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n"), title
}
