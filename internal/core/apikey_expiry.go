package core

import (
	"encoding/json"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// expiryChange is a PATCH's expires_at: absent leaves the key's date alone, null means the key
// never expires, a date replaces the current one.
type expiryChange struct {
	Set   bool
	Value *time.Time
}

func (e *expiryChange) UnmarshalJSON(b []byte) error {
	e.Set = true
	if string(b) == "null" {
		e.Value = nil
		return nil
	}
	var t time.Time
	if err := json.Unmarshal(b, &t); err != nil {
		return err
	}
	e.Value = &t
	return nil
}

// Schema is a nullable date-time.
func (expiryChange) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Type: huma.TypeString, Format: "date-time", Nullable: true,
		Description: "When the key stops working, in the future, or null for never. A later date or null needs a recent sign-in (and the second factor when on), as creating a key does; an earlier one applies at once"}
}

// expiryWidens reports whether moving a key's expiry from `from` to `to` keeps it working longer:
// a later date, or no date at all where it had one. nil means never.
func expiryWidens(from, to *time.Time) bool {
	switch {
	case to == nil:
		return from != nil
	case from == nil:
		return false
	default:
		return to.After(*from)
	}
}

// sameExpiry reports whether two expiries are the same (both never counts).
func sameExpiry(a, b *time.Time) bool {
	return (a == nil) == (b == nil) && (a == nil || a.Equal(*b))
}
