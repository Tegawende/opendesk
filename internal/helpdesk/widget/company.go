package widget

import (
	"context"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/contacts"
	"github.com/useopendesk/opendesk/internal/ids"
)

// A user_hash proves who the user is, not which account they belong to, so on its own the
// page's company is a claim: it links the contact to a company the workspace already has
// (linkCompany) and nothing more. company_hash is the host's server vouching for the pair:
//
//	company_hash = hex HMAC-SHA256(widget_secret, user_id + "\n" + company.id)
//
// With it, identify also creates the company when the workspace has none with that external
// id, and fills the company's fields whose key starts with FieldPrefix. The message holds the
// user id so that a hash vouches for one member of the account, not for whoever has seen it, and
// so that it is never a user_hash: a hash of the company id alone would be the user_hash of the
// user with the same id, wherever the host's user and account ids overlap. A line break joins
// the two because a company id holding a control character is never vouched for.

// companyHashMessage is what company_hash signs.
func companyHashMessage(userID, companyID string) string { return userID + "\n" + companyID }

// vouchedCompanyID is the page's company id when company_hash vouches for this user and this
// id, else "". A missing or wrong hash is not an error: the company then stays a claim.
func (h *handler) vouchedCompanyID(w widgetRow, userID string, company *widgetCompany, hash string) (string, error) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if company == nil || hash == "" || len(w.SecretEnc) == 0 {
		return "", nil
	}
	id := strings.TrimSpace(company.ID)
	if id == "" || len([]rune(id)) > 200 || strings.IndexFunc(id, unicode.IsControl) >= 0 {
		return "", nil
	}
	secret, err := h.d.Box.Open(w.SecretEnc)
	if err != nil {
		return "", err
	}
	if !crypto.EqualHex(crypto.HMACHex(secret, companyHashMessage(userID, id)), hash) {
		return "", nil
	}
	return id, nil
}

// syncCompany applies a company the host vouched for: it creates the company the first time,
// fills its opted-in fields, and links the contact to it. Without a valid company_hash it does
// nothing, and syncProfile's link is all the company gets. A value that does not fit is
// skipped, so one bad attribute never keeps a visitor from the chat.
func (h *handler) syncCompany(ctx context.Context, tx pgx.Tx, w widgetRow, contactID uuid.UUID, userID string, company *widgetCompany, hash string) error {
	externalID, err := h.vouchedCompanyID(w, userID, company, hash)
	if err != nil || externalID == "" {
		return err
	}
	wid := w.WorkspaceID
	companyID, created, err := ensureCompany(ctx, tx, wid, externalID, company)
	if err != nil || companyID == uuid.Nil {
		return err
	}
	fields, _ := company.Fields.(map[string]any)
	set, err := setCompanyFields(ctx, tx, wid, companyID, fields)
	if err != nil {
		return err
	}
	switch {
	case created:
		err = contacts.PublishCompany(ctx, tx, wid, companyID, "company.created")
	case set:
		err = contacts.PublishCompany(ctx, tx, wid, companyID, "company.updated")
	}
	if err != nil {
		return err
	}
	linked, err := linkCompany(ctx, tx, wid, contactID, externalID)
	if err != nil || !linked {
		return err
	}
	return publishContact(ctx, tx, wid, contactID, "contact.updated")
}

// ensureCompany finds the workspace's company with this external id, or creates it with the
// page's name (else the id) and domain (unless another company has it). An existing company's
// name and domain are the team's: identify never changes them. It answers uuid.Nil when
// neither can name the company, or when a concurrent write took the id or the domain first;
// the next identify finds it.
func ensureCompany(ctx context.Context, tx pgx.Tx, wid uuid.UUID, externalID string, in *widgetCompany) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM companies WHERE workspace_id = $1 AND external_id = $2`, wid, externalID).Scan(&id)
	if !db.IsNotFound(err) {
		return id, false, err
	}
	name := contacts.CompanyName(text(in.Name))
	if name == "" {
		name = contacts.CompanyName(externalID)
	}
	if name == "" {
		return uuid.Nil, false, nil
	}
	id = ids.New()
	tag, err := tx.Exec(ctx, `INSERT INTO companies (id, workspace_id, name, domain, external_id)
		SELECT $1, $2, $3, CASE WHEN $4 = '' OR EXISTS (SELECT 1 FROM companies WHERE workspace_id = $2 AND domain = $4) THEN '' ELSE $4 END, $5
		ON CONFLICT DO NOTHING`, id, wid, name, contacts.CompanyDomain(text(in.Domain)), externalID)
	if err != nil {
		return uuid.Nil, false, err
	}
	if tag.RowsAffected() > 0 {
		return id, true, nil
	}
	err = tx.QueryRow(ctx, `SELECT id FROM companies WHERE workspace_id = $1 AND external_id = $2`, wid, externalID).Scan(&id)
	if db.IsNotFound(err) {
		return uuid.Nil, false, nil
	}
	return id, false, err
}

// setCompanyFields writes the opted-in company fields the page sent and reports whether any changed.
func setCompanyFields(ctx context.Context, tx pgx.Tx, wid, companyID uuid.UUID, in map[string]any) (bool, error) {
	if len(in) == 0 {
		return false, nil
	}
	defs, err := domain.LoadFieldDefs(ctx, tx, wid, domain.FieldEntityCompany)
	if err != nil {
		return false, err
	}
	var cur map[string]any
	if err := tx.QueryRow(ctx, `SELECT fields FROM companies WHERE workspace_id = $1 AND id = $2`, wid, companyID).Scan(&cur); err != nil {
		return false, err
	}
	next, changed := widgetFields(defs, cur, in)
	if !changed {
		return false, nil
	}
	_, err = tx.Exec(ctx, `UPDATE companies SET fields = $3, updated_at = now() WHERE workspace_id = $1 AND id = $2`, wid, companyID, next)
	return true, err
}

// text is v when the page sent a string, else "".
func text(v any) string {
	s, _ := v.(string)
	return s
}
