package resources

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// ViewIcons are the icons a view can wear (kind "icon"), by their lucide name. The web
// app's copy, with the components it renders, is web/src/components/tickets/viewIcons.ts;
// TestViewIconNames keeps the two the same.
var ViewIcons = []string{
	"inbox", "mail", "send", "archive", "message-circle", "messages-square", "phone", "at-sign", "megaphone", "bell",
	"package", "package-check", "package-x", "box", "truck", "undo-2", "repeat", "warehouse", "map-pin",
	"shopping-cart", "shopping-bag", "store", "credit-card", "receipt", "banknote", "euro", "dollar-sign", "badge-percent", "tag", "gift",
	"triangle-alert", "circle-alert", "shield-alert", "siren", "flame", "zap", "clock", "hourglass", "timer", "calendar", "circle-pause", "circle-check", "ban",
	"user", "users", "user-check", "user-plus", "building-2", "briefcase", "handshake", "crown", "heart", "smile", "frown", "thumbs-up", "thumbs-down",
	"bug", "wrench", "settings", "smartphone", "plug", "key", "lock", "lightbulb",
	"layers", "star", "flag", "bookmark", "sparkles", "globe", "languages", "file-text", "folder", "scale", "graduation-cap", "eye", "search",
}

// ViewIcon is what a view shows beside its name in the sidebar: an emoji, an icon of
// the list, or one or two typed characters, the last two in a palette colour if set.
type ViewIcon struct {
	Kind  string  `json:"kind" enum:"emoji,icon,text" doc:"emoji: one emoji; icon: an icon of the list, by name; text: one or two letters or digits"`
	Value string  `json:"value" doc:"The emoji, the icon's name or the characters"`
	Color *string `json:"color" doc:"A palette colour for an icon or text; always null for an emoji"`
}

// viewIconIn is the body's icon: absent keeps it, null clears it.
type viewIconIn struct {
	Set, Null bool
	Value     struct {
		Kind  string  `json:"kind"`
		Value string  `json:"value"`
		Color *string `json:"color"`
	}
}

func (n *viewIconIn) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Null = true
		return nil
	}
	return json.Unmarshal(b, &n.Value)
}

// Schema describes the field as a nullable icon object.
func (viewIconIn) Schema(huma.Registry) *huma.Schema {
	str := func(doc string) *huma.Schema { return &huma.Schema{Type: huma.TypeString, Description: doc} }
	color := str("A palette colour for an icon or text (ignored for an emoji); null for none")
	color.Nullable = true
	return &huma.Schema{
		Type:        huma.TypeObject,
		Nullable:    true,
		Description: "One emoji, an icon of the list or one or two letters or digits, with an optional colour; null shows the default icon",
		Properties: map[string]*huma.Schema{
			"kind":  {Type: huma.TypeString, Enum: []any{"emoji", "icon", "text"}},
			"value": str("The emoji, the icon's name or the characters"),
			"color": color,
		},
		Required: []string{"kind", "value"},
	}
}

// cleanViewIcon checks an icon: one emoji (its colour dropped), a listed icon, or one
// or two letters or digits; a colour is one of the labels' palette.
func cleanViewIcon(in viewIconIn) (*ViewIcon, error) {
	if !in.Set || in.Null {
		return nil, nil
	}
	v := strings.TrimSpace(in.Value.Value)
	out := ViewIcon{Kind: in.Value.Kind, Value: v}
	switch in.Value.Kind {
	case "emoji":
		if len(v) > 64 || !hd.IsEmoji(v) {
			return nil, httpx.Validation("icon.value must be one emoji")
		}
		return &out, nil
	case "icon":
		if !slices.Contains(ViewIcons, v) {
			return nil, httpx.Validation("icon.value is not an icon of the list")
		}
	case "text":
		if n := utf8.RuneCountInString(v); n < 1 || n > 2 {
			return nil, httpx.Validation("icon.value must be one or two letters or digits")
		}
		for _, r := range v {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				return nil, httpx.Validation("icon.value must be one or two letters or digits")
			}
		}
	default:
		return nil, httpx.Validation("icon.kind must be emoji, icon or text")
	}
	if c := in.Value.Color; c != nil && strings.TrimSpace(*c) != "" {
		color, err := checkColor("icon.color", *c)
		if err != nil {
			return nil, err
		}
		out.Color = &color
	}
	return &out, nil
}
