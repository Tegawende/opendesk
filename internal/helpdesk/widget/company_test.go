package widget_test

import (
	"testing"

	"github.com/useopendesk/opendesk/internal/crypto"
)

// TestSessionCompanyHash: a company_hash vouching for the user and the company lets identify
// create the company and set its widget_ fields; without it, or with a hash for anything else,
// the company stays a claim that only links.
func TestSessionCompanyHash(t *testing.T) {
	e, ws := setup(t)
	secret := []byte("s3cret")
	e.Exec(`INSERT INTO custom_fields (workspace_id, entity, key, name, type) VALUES
		($1, 'company', 'widget_tier', 'Tier', 'text'),
		($1, 'company', 'widget_seats', 'Seats', 'number'),
		($1, 'company', 'region', 'Region', 'text'),
		($1, 'contact', 'widget_plan', 'Plan', 'text')`, ws.ID)
	e.Exec(`INSERT INTO companies (workspace_id, name, domain, external_id) VALUES ($1, 'Acme Ltd', 'acme.example', 'acct-9')`, ws.ID)
	type company struct {
		Name, Domain string
		Fields       map[string]any
	}
	load := func(externalID string) *company {
		var n int
		e.Scalar(`SELECT count(*) FROM companies WHERE workspace_id = $1 AND external_id = $2`, &n, ws.ID, externalID)
		if n == 0 {
			return nil
		}
		var c company
		e.Scalar(`SELECT name FROM companies WHERE workspace_id = $1 AND external_id = $2`, &c.Name, ws.ID, externalID)
		e.Scalar(`SELECT domain::text FROM companies WHERE workspace_id = $1 AND external_id = $2`, &c.Domain, ws.ID, externalID)
		e.Scalar(`SELECT fields FROM companies WHERE workspace_id = $1 AND external_id = $2`, &c.Fields, ws.ID, externalID)
		return &c
	}
	linked := func(contactID any) string {
		var ext *string
		e.Scalar(`SELECT co.external_id FROM contacts c LEFT JOIN companies co ON co.id = c.company_id WHERE c.id = $1`, &ext, contactID)
		if ext == nil {
			return ""
		}
		return *ext
	}
	count := func(typ string) int {
		var n int
		e.Scalar(`SELECT count(*) FROM events WHERE workspace_id = $1 AND type = $2`, &n, ws.ID, typ)
		return n
	}
	identify := func(userID string, extra map[string]any) map[string]any {
		body := map[string]any{"key": ws.WidgetKey, "user_id": userID, "user_hash": crypto.HMACHex(secret, userID)}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}

	// A vouched id the workspace does not have creates the company, named by the page, with the
	// opted-in fields only, and links the contact to it.
	body := identify("user-1", map[string]any{
		"company_hash": crypto.HMACHex(secret, "user-1\nacct-new"),
		"company": map[string]any{"id": " acct-new ", "name": "  New   Co ", "domain": "https://www.newco.example/",
			"fields": map[string]any{"widget_tier": "gold", "widget_seats": 40, "region": "eu", "widget_unknown": "x"}},
		"fields": map[string]any{"widget_plan": "pro"},
	})
	s := start(t, e, body)
	c := load("acct-new")
	if c == nil || c.Name != "New Co" || c.Domain != "newco.example" {
		t.Fatalf("created %+v", c)
	}
	if c.Fields["widget_tier"] != "gold" || c.Fields["widget_seats"] != float64(40) || len(c.Fields) != 2 {
		t.Fatalf("company fields %v", c.Fields)
	}
	if got := linked(s.Contact.ID); got != "acct-new" {
		t.Fatalf("linked to %q", got)
	}
	var contactFields map[string]any
	e.Scalar(`SELECT fields FROM contacts WHERE id = $1`, &contactFields, s.Contact.ID)
	if contactFields["widget_plan"] != "pro" || len(contactFields) != 1 {
		t.Fatalf("contact fields %v", contactFields)
	}
	if count("company.created") != 1 || count("contact.updated") == 0 {
		t.Fatalf("events: %d company.created, %d contact.updated", count("company.created"), count("contact.updated"))
	}

	// Again on the same session: the fields follow the page, the name and domain stay as created.
	body["visitor_token"] = s.VisitorToken
	body["company"] = map[string]any{"id": "acct-new", "name": "Renamed", "domain": "other.example", "fields": map[string]any{"widget_tier": "platinum"}}
	again := start(t, e, body)
	c = load("acct-new")
	if again.Contact.ID != s.Contact.ID || c.Name != "New Co" || c.Domain != "newco.example" || c.Fields["widget_tier"] != "platinum" || c.Fields["widget_seats"] != float64(40) {
		t.Fatalf("update: %+v", c)
	}
	if count("company.created") != 1 || count("company.updated") != 1 {
		t.Fatalf("events: %d company.created, %d company.updated", count("company.created"), count("company.updated"))
	}
	// Nothing new: nothing announced.
	start(t, e, body)
	if count("company.updated") != 1 {
		t.Fatal("an unchanged company was announced")
	}

	// A company the team made keeps its name and domain; only its widget_ fields are set.
	u2 := start(t, e, identify("user-2", map[string]any{
		"company_hash": crypto.HMACHex(secret, "user-2\nacct-9"),
		"company":      map[string]any{"id": "acct-9", "name": "Evil", "domain": "evil.example", "fields": map[string]any{"widget_tier": "silver", "region": "us"}},
	}))
	c = load("acct-9")
	if linked(u2.Contact.ID) != "acct-9" || c.Name != "Acme Ltd" || c.Domain != "acme.example" || c.Fields["widget_tier"] != "silver" || len(c.Fields) != 1 {
		t.Fatalf("existing: %+v", c)
	}

	// A domain another company has is left off the new one; without a name it is named by its id.
	start(t, e, identify("user-3", map[string]any{
		"company_hash": crypto.HMACHex(secret, "user-3\nacct-3"),
		"company":      map[string]any{"id": "acct-3", "domain": "acme.example"},
	}))
	if c = load("acct-3"); c == nil || c.Name != "acct-3" || c.Domain != "" {
		t.Fatalf("taken domain: %+v", c)
	}

	// Values of the wrong type are ignored, and the session still starts.
	start(t, e, identify("user-4", map[string]any{
		"company_hash": crypto.HMACHex(secret, "user-4\nacct-4"),
		"company":      map[string]any{"id": "acct-4", "name": 42, "domain": true, "fields": "gold"},
	}))
	if c = load("acct-4"); c == nil || c.Name != "acct-4" || c.Domain != "" || len(c.Fields) != 0 {
		t.Fatalf("wrong types: %+v", c)
	}

	// Hashes that do not vouch for this user and this company: another user's, the company id's
	// alone (which is the user_hash of a user with that id), one keyed with another secret, and an
	// id with a control character. A known id still links, as without a hash; nothing is created
	// or written.
	tierBefore := load("acct-9").Fields["widget_tier"]
	for name, hash := range map[string]string{
		"other user":   crypto.HMACHex(secret, "user-1\nacct-9"),
		"company only": crypto.HMACHex(secret, "acct-9"),
		"other secret": crypto.HMACHex([]byte("other"), "user-5\nacct-9"),
	} {
		u := start(t, e, identify("user-5", map[string]any{"company_hash": hash,
			"company": map[string]any{"id": "acct-9", "fields": map[string]any{"widget_tier": "hacked"}}}))
		if linked(u.Contact.ID) != "acct-9" || load("acct-9").Fields["widget_tier"] != tierBefore {
			t.Fatalf("%s: wrote %v", name, load("acct-9").Fields)
		}
		start(t, e, identify("user-5", map[string]any{"company_hash": hash,
			"company": map[string]any{"id": "acct-ghost", "name": "Ghost"}}))
		if load("acct-ghost") != nil {
			t.Fatalf("%s: created a company", name)
		}
	}
	start(t, e, identify("user-6", map[string]any{"company_hash": crypto.HMACHex(secret, "user-6\nacct\x01x"),
		"company": map[string]any{"id": "acct\x01x", "name": "Control"}}))
	if load("acct\x01x") != nil {
		t.Fatal("a control character in the id created a company")
	}

	// The same id as both a user and an account: that user's user_hash is no company_hash.
	e.Exec(`INSERT INTO companies (workspace_id, name, external_id) VALUES ($1, 'Account 77', '77')`, ws.ID)
	start(t, e, identify("77", map[string]any{"company_hash": crypto.HMACHex(secret, "77"),
		"company": map[string]any{"id": "77", "fields": map[string]any{"widget_tier": "hacked"}}}))
	if f := load("77").Fields; len(f) != 0 {
		t.Fatalf("a user_hash wrote company fields: %v", f)
	}

	// Without a verified identity a company_hash does nothing.
	anon := start(t, e, map[string]any{"key": ws.WidgetKey, "company_hash": crypto.HMACHex(secret, "\nacct-anon"),
		"company": map[string]any{"id": "acct-anon", "name": "Anon"}})
	if load("acct-anon") != nil || linked(anon.Contact.ID) != "" {
		t.Fatal("an anonymous visitor created or linked a company")
	}
}
