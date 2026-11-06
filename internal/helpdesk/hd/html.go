// Package hd holds what the helpdesk subpackages share: HTML sanitizing, workspace
// settings, notifications, reference checks, validation of filters and automations,
// and attachment uploads. Every query is scoped by the caller's workspace id.
package hd

import (
	"context"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"

	"github.com/useopendesk/opendesk/internal/config"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/templates"
)

var (
	uuidAttr = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	// Images load from http(s) or from our own attachment route (relative or absolute).
	imgSrc = regexp.MustCompile(`^(?i:https?://[^\s"'<>]+|/v1/attachments/[0-9a-fA-F-]{36}(\?[^\s"'<>]*)?)$`)

	skipContent = []string{"style", "script", "head", "title", "noscript", "iframe", "object", "embed", "applet", "svg", "math", "template"}

	richPolicy = newRichPolicy()
	textPolicy = bluemonday.StrictPolicy().SkipElementsContent(skipContent...)
	blockBreak = regexp.MustCompile(`(?i)<(br|/p|/div|/tr|/li|/h[1-6]|/blockquote|/pre)\s*/?>`)
	tagPattern = regexp.MustCompile(`(?i)<(!doctype|/?[a-z][a-z0-9]*)(\s|>|/>)`)
)

// newRichPolicy is the allowlist for HTML that agents write (replies, notes, saved
// replies, articles, automation replies). Nothing that can execute survives.
func newRichPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.SkipElementsContent(skipContent...)
	p.AllowElements("p", "br", "strong", "em", "u", "s", "ul", "ol", "li", "blockquote", "code", "pre", "h1", "h2", "h3", "span")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.AllowRelativeURLs(true)
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.AllowAttrs("src").Matching(imgSrc).OnElements("img")
	p.AllowAttrs("alt").OnElements("img")
	// Mentions: the editor's <span data-type="mention" data-id="<user id>" class="mention">,
	// and the older data-mention="<user id>".
	p.AllowAttrs("data-mention").Matching(uuidAttr).OnElements("span", "a")
	p.AllowAttrs("data-type", "class").Matching(regexp.MustCompile(`^mention$`)).OnElements("span")
	p.AllowAttrs("data-id").Matching(uuidAttr).OnElements("span")
	return p
}

// emailPolicy is the policy of every stored email body: no scripts, event handlers, forms
// or frames; layout attributes and email CSS (css.go) stay; links get rel="nofollow
// noopener" and open in a new tab; images are allowed (http, https and data: images).
// Signatures and auto-replies use the template design allowlist (SanitizeSignature).
var emailPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowRelativeURLs(false)
	p.RequireNoFollowOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.AllowDataURIImages()
	p.AllowElements("center", "font")
	p.AllowAttrs("align").Matching(regexp.MustCompile(`(?i)^(left|right|center|justify)$`)).OnElements("p", "div", "td", "th", "tr", "table", "img", "h1", "h2", "h3", "h4", "h5", "h6")
	p.AllowAttrs("valign").Matching(bluemonday.CellVerticalAlign).OnElements("table", "tr", "td", "th", "img")
	p.AllowAttrs("width", "height").Matching(bluemonday.NumberOrPercent).OnElements("table", "td", "th", "img", "tr")
	p.AllowAttrs("cellpadding", "cellspacing", "border").Matching(bluemonday.Integer).OnElements("table")
	p.AllowAttrs("bgcolor").Matching(cssColor).OnElements("table", "tr", "td", "th")
	p.AllowAttrs("color").Matching(cssColor).OnElements("font")
	p.AllowAttrs("size").Matching(regexp.MustCompile(`^[+-]?[1-7]$`)).OnElements("font")
	p.AllowAttrs("face").Matching(regexp.MustCompile(`^[\p{L}\p{N} ,'"_-]{1,200}$`)).OnElements("font")
	p.AllowAttrs("dir").Matching(bluemonday.Direction).Globally()
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^[\p{L}\p{N}_ -]{1,500}$`)).Globally()
	for prop := range emailCSSProps {
		p.AllowStyles(prop).MatchingHandler(emailCSSValue).Globally()
	}
	return p
}()

var cssColor = regexp.MustCompile(`(?i)^(#[0-9a-f]{3,8}|[a-z]{3,20})$`)

// SanitizeEmailHTML is the only way email HTML reaches the database. The email's style
// sheets, cleaned, lead the body.
func SanitizeEmailHTML(raw string) string {
	body := dropEscapedStyles(strings.TrimSpace(emailPolicy.Sanitize(raw)))
	if sheets := emailStyleSheets(raw); sheets != "" && body != "" {
		return "<style>" + sheets + "</style>" + body
	}
	return body
}

// SanitizeForwardedEmail is SanitizeEmailHTML for an email placed inside another (a forward):
// the same policy, without the email's style sheets, which would restyle the message, the
// signature and the layout around it. Its inline styles and layout attributes stay.
func SanitizeForwardedEmail(raw string) string {
	return dropEscapedStyles(strings.TrimSpace(emailPolicy.Sanitize(raw)))
}

// SanitizeSignature cleans a signature, auto-reply or other designed email part with the
// design allowlist, keeping its template tags (contracts/templates.md).
func SanitizeSignature(raw string) string { return templates.SanitizeDesign(raw) }

// SanitizeRich returns agent HTML reduced to the rich-text allowlist.
func SanitizeRich(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	return strings.TrimSpace(richPolicy.Sanitize(raw))
}

// SanitizeRichTemplate is SanitizeRich keeping template tags, also inside links: saved
// replies and replies before they are filled.
func SanitizeRichTemplate(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	return strings.TrimSpace(templates.Protect(raw, richPolicy.Sanitize))
}

// HTMLToText flattens HTML to readable plain text with line structure kept.
func HTMLToText(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if !tagPattern.MatchString(raw) {
		return compactLines(html.UnescapeString(raw))
	}
	withBreaks := blockBreak.ReplaceAllString(raw, "\n$0")
	return compactLines(html.UnescapeString(textPolicy.Sanitize(withBreaks)))
}

// TextToHTML turns plain text (widget messages) into paragraphs, escaped.
func TextToHTML(text string) string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n")
	if text == "" {
		return ""
	}
	var b strings.Builder
	for _, para := range regexp.MustCompile(`\n{2,}`).Split(text, -1) {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		b.WriteString("<p>")
		b.WriteString(strings.ReplaceAll(html.EscapeString(para), "\n", "<br>"))
		b.WriteString("</p>")
	}
	return b.String()
}

// Excerpt is the first n runes of text on one line, cut at a word when possible.
func Excerpt(text string, n int) string {
	s := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)[:n]
	out := string(r)
	if i := strings.LastIndex(out, " "); i > n/2 {
		out = out[:i]
	}
	return strings.TrimRight(out, " ,.;:") + "…"
}

// Truncate cuts s to n runes.
func Truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func compactLines(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, ln := range lines {
		ln = strings.Join(strings.Fields(ln), " ")
		if ln == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		blank = false
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// FillReply sanitizes a reply and fills its template tags for ref's ticket
// (contracts/templates.md): the stored reply is what the customer gets. Tags it does not
// know stay as written.
func FillReply(ctx context.Context, q db.Querier, cfg *config.Config, ref templates.Ref, raw string) (string, error) {
	if !templates.HasTags(raw) {
		return SanitizeRich(raw), nil
	}
	c, err := templates.Load(ctx, q, cfg, ref)
	if err != nil {
		return "", err
	}
	return SanitizeRich(templates.Render(SanitizeRichTemplate(raw), c.Values, templates.HTML)), nil
}
