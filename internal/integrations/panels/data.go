package panels

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/domain"
)

// Data is what a panel shows for one ticket. Commerce panels (shopify) fill Customer and
// Orders, the same shape whatever the store, so another commerce source plugs into the
// same section; http panels fill Sections.
type Data struct {
	PanelID   uuid.UUID `json:"panel_id"`
	Kind      string    `json:"kind" enum:"shopify,http"`
	State     string    `json:"state" enum:"ok,empty,no_contact,error" doc:"empty: the product knows nothing about this contact; no_contact: the ticket's contact has no email"`
	Message   string    `json:"message,omitempty" doc:"error: what went wrong, in a few words"`
	Customer  *Customer `json:"customer,omitempty"`
	Orders    []Order   `json:"orders,omitempty"`
	Sections  []Section `json:"sections,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
}

// Customer is the store's own record of the contact. Amounts are decimal strings.
type Customer struct {
	Name        string   `json:"name"`
	URL         string   `json:"url" doc:"The customer in the store's admin"`
	OrdersCount int64    `json:"orders_count"`
	TotalSpent  string   `json:"total_spent"`
	Currency    string   `json:"currency"`
	Tags        []string `json:"tags"`
}

// Order is one of the contact's last orders, newest first.
type Order struct {
	ID                string      `json:"id"`
	Number            string      `json:"number" doc:"As the store shows it, such as #1042"`
	URL               string      `json:"url" doc:"The order in the store's admin"`
	CreatedAt         time.Time   `json:"created_at"`
	Total             string      `json:"total"`
	Currency          string      `json:"currency"`
	FinancialStatus   string      `json:"financial_status" doc:"Lowercase, such as paid, pending, refunded, partially_refunded"`
	FulfillmentStatus string      `json:"fulfillment_status" doc:"Lowercase, such as fulfilled, unfulfilled, partially_fulfilled"`
	Cancelled         bool        `json:"cancelled"`
	Items             []OrderItem `json:"items"`
	Tracking          []Tracking  `json:"tracking"`
}

// OrderItem is one line of an order.
type OrderItem struct {
	Title    string `json:"title"`
	Variant  string `json:"variant,omitempty"`
	Quantity int    `json:"quantity"`
	Price    string `json:"price" doc:"Unit price"`
}

// Tracking is one parcel of an order.
type Tracking struct {
	Company string `json:"company,omitempty"`
	Number  string `json:"number,omitempty"`
	URL     string `json:"url,omitempty"`
}

// Section is a titled list of labelled values, as an http panel answers it.
type Section struct {
	Title string        `json:"title"`
	Items []SectionItem `json:"items"`
}

// SectionItem is one value; a URL makes it a link.
type SectionItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
	URL   string `json:"url,omitempty"`
}

// subject is what a lookup is about: the ticket and its contact.
type subject struct {
	TicketID     uuid.UUID
	TicketNumber int64
	Subject      string
	ContactID    *uuid.UUID
	Email        string
	Name         string
}

func subjectOf(t *domain.Ticket) subject {
	sub := subject{TicketID: t.ID, TicketNumber: t.Number, Subject: t.Subject}
	if t.Contact != nil {
		id := t.Contact.ID
		sub.ContactID, sub.Name = &id, t.Contact.Name
		if t.Contact.Email != nil {
			sub.Email = strings.ToLower(strings.TrimSpace(*t.Contact.Email))
		}
	}
	return sub
}

// read answers the panel for the ticket: from the cache when it answered in the last
// minute (a failure, the last 15 seconds), else from the product, within fetchTimeout.
func (s *Service) read(ctx context.Context, x *row, t *domain.Ticket, refresh bool) Data {
	sub := subjectOf(t)
	base := Data{PanelID: x.ID, Kind: x.Kind, FetchedAt: time.Now().UTC()}
	if x.Kind == KindShopify && sub.Email == "" {
		base.State = "no_contact"
		return base
	}
	key := cacheKey{panel: x.ID, version: x.UpdatedAt.UnixNano(), email: sub.Email}
	if x.Kind == KindHTTP {
		// The URL is told the ticket too, so its answer may differ per ticket.
		key.ticket = sub.TicketID
	}
	if !refresh {
		if d, ok := s.cache.get(key); ok {
			return d
		}
	}
	if !s.d.Limiter.Allow("ticket-panel:lookup:"+x.ID.String(), lookupsPerMinute, time.Minute) {
		base.State, base.Message = "error", "Too many lookups. Try again in a minute."
		return base
	}
	lctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	var err error
	switch x.Kind {
	case KindShopify:
		err = s.readShopify(lctx, x, sub, &base)
	case KindHTTP:
		err = s.readHTTP(lctx, x, sub, &base)
	}
	ttl := cacheTTL
	if err != nil {
		base.State, base.Message, base.Customer, base.Orders, base.Sections = "error", lookupMessage(err), nil, nil, nil
		ttl = errTTL
		s.d.Log.Info("ticket panel lookup failed", "panel", x.ID, "kind", x.Kind, "host", x.api().Host, "err", lookupMessage(err))
	}
	// A refused credential, or a lookup that works again, shows in settings (a write only
	// when the status changes).
	s.markStatus(context.WithoutCancel(ctx), x, err)
	s.cache.set(key, base, ttl)
	return base
}

// lookupMessage is what an agent reads when a lookup fails: the product's words or how
// reaching it failed, never a URL (a path or query can carry a secret).
func lookupMessage(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "No answer in time."
	case errors.Is(err, errAuth):
		return clip(err.Error(), 200)
	}
	msg := err.Error()
	if i := strings.Index(msg, ": "); i >= 0 && strings.Contains(msg[:i], "://") {
		msg = msg[i+2:] // *url.Error: drop "Post \"https://…\""
	}
	return clip(msg, 200)
}

// cacheKey is one lookup: a panel at a version (a changed panel never answers from
// before the change), for an email (and a ticket, for http panels).
type cacheKey struct {
	panel   uuid.UUID
	version int64
	email   string
	ticket  uuid.UUID
}

type cached struct {
	data    Data
	expires time.Time
}

// answerCache keeps recent answers in this process. Several API processes each keep
// their own; an answer is at most a minute old either way.
type answerCache struct {
	mu  sync.Mutex
	max int
	m   map[cacheKey]cached
	now func() time.Time
}

func newAnswerCache(max int) *answerCache {
	return &answerCache{max: max, m: map[cacheKey]cached{}, now: time.Now}
}

func (c *answerCache) get(k cacheKey) (Data, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	if !ok || c.now().After(v.expires) {
		return Data{}, false
	}
	return v.data, true
}

func (c *answerCache) set(k cacheKey, d Data, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= c.max {
		now := c.now()
		for key, v := range c.m {
			if now.After(v.expires) {
				delete(c.m, key)
			}
		}
		if len(c.m) >= c.max {
			c.m = map[cacheKey]cached{}
		}
	}
	c.m[k] = cached{data: d, expires: c.now().Add(ttl)}
}

// forget drops a removed panel's answers.
func (c *answerCache) forget(panel uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.m {
		if key.panel == panel {
			delete(c.m, key)
		}
	}
}
