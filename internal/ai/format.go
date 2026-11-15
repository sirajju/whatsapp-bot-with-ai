package ai

import (
	"html"
	"regexp"
	"strings"
)

// Model output cleanup. Even with instructions, models occasionally emit
// Markdown or HTML. FormatForWhatsApp converts the common offenders into
// WhatsApp-native syntax. Idempotent on already-clean text.

var (
	reMDBoldStar = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)       // **x** → *x*
	reMDBoldUnd  = regexp.MustCompile(`__([^_\n]+)__`)           // __x__ → *x*
	reMDHeading  = regexp.MustCompile(`(?m)^#{1,6}\s*`)          // strip "# " at line start
	reHTMLBreak  = regexp.MustCompile(`(?i)<br\s*/?>`)           // <br> → \n
	reHTMLTag    = regexp.MustCompile(`</?(?:p|div|span|b|i|em|strong|code|pre|ul|ol|li|h[1-6])\b[^>]*>`)
	reMDBullet   = regexp.MustCompile(`(?m)^(\s*)[-*]\s+`)       // "- item" → "• item"
	reMDLink     = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`) // [text](url) → text (url)
)

// FormatForWhatsApp normalises a model reply for a WhatsApp message.
func FormatForWhatsApp(s string) string {
	if s == "" {
		return s
	}
	s = reHTMLBreak.ReplaceAllString(s, "\n")
	s = reHTMLTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = reMDHeading.ReplaceAllString(s, "")
	s = reMDBoldStar.ReplaceAllString(s, "*$1*")
	s = reMDBoldUnd.ReplaceAllString(s, "*$1*")
	s = reMDBullet.ReplaceAllString(s, "$1• ")
	s = reMDLink.ReplaceAllString(s, "$1 ($2)")
	return strings.TrimSpace(s)
}
