package relay

import (
	"html"
	"regexp"
	"strings"
)

var (
	invisibleBlocks = regexp.MustCompile(`(?is)<(head|style|script|title)\b.*?</(head|style|script|title)\s*>`)
	anchors         = regexp.MustCompile(`(?is)<a\b[^>]*?href\s*=\s*["']([^"']+)["'][^>]*>(.*?)</a\s*>`)
	lineBreaks      = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|h[1-6]|li|tr|table|blockquote)\s*>`)
	tags            = regexp.MustCompile(`(?s)<[^>]*>`)
	spaces          = regexp.MustCompile(`[ \t\f\v\x{00a0}]+`)
	blankLines      = regexp.MustCompile(`\n{3,}`)
)

func PlainText(body string) string {
	s := invisibleBlocks.ReplaceAllString(body, "")
	s = anchors.ReplaceAllStringFunc(s, func(a string) string {
		m := anchors.FindStringSubmatch(a)
		label := strings.TrimSpace(tags.ReplaceAllString(m[2], ""))
		if label == "" || label == m[1] {
			return m[1]
		}
		return label + " (" + m[1] + ")"
	})
	s = lineBreaks.ReplaceAllString(s, "\n")
	s = html.UnescapeString(tags.ReplaceAllString(s, ""))
	lines := strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(spaces.ReplaceAllString(line, " "))
	}
	return strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")) + "\n"
}
