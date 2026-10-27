package panels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/useopendesk/opendesk/internal/version"
)

const (
	// shopifyVersion is the Admin API version the panel speaks. Shopify supports each for a
	// year; a new one comes out every quarter (shopify.dev/docs/api/usage/versioning).
	shopifyVersion = "2026-10"
	// shopifyOrders and shopifyItems bound what one lookup reads.
	shopifyOrders = 5
	shopifyItems  = 10
	// maxShopifyAnswer bounds an Admin API answer.
	maxShopifyAnswer = 1 << 20
)

var shopRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,59}\.myshopify\.com$`)

// normalizeShop accepts a shop's handle, its myshopify.com domain or an admin URL, and
// answers the domain.
func normalizeShop(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		s = u.Host
	}
	s = strings.TrimSuffix(s, "/")
	if s != "" && !strings.Contains(s, ".") {
		s += ".myshopify.com"
	}
	if !shopRe.MatchString(s) {
		return "", errors.New("Use the shop's myshopify.com domain, such as my-store.myshopify.com.")
	}
	return s, nil
}

func shopifyOrigin(shop string) string { return "https://" + shop }

// shopHandle is the shop's name in admin.shopify.com links.
func shopHandle(shop string) string { return strings.TrimSuffix(shop, ".myshopify.com") }

// adminURL is the shop's admin, where its customers and orders open.
func adminURL(shop string) string {
	return "https://admin.shopify.com/store/" + url.PathEscape(shopHandle(shop))
}

// positive reports whether a decimal amount is above zero.
func positive(amount string) bool {
	f, err := strconv.ParseFloat(strings.TrimSpace(amount), 64)
	return err == nil && f > 0
}

// gidNumber is the number at the end of a Shopify global id (gid://shopify/Order/123).
func gidNumber(gid string) string {
	if i := strings.LastIndexByte(gid, '/'); i >= 0 {
		return gid[i+1:]
	}
	return gid
}

// graphql posts one Admin API query and decodes its data into out.
func (s *Service) graphql(ctx context.Context, shop, token, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	endpoint := s.shopifyBase(shop) + "/admin/api/" + shopifyVersion + "/graphql.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "opendesk-panels/"+version.Version)
	req.Header.Set("X-Shopify-Access-Token", token)
	res, err := s.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // never the URL
		}
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxShopifyAnswer+1))
	if err != nil {
		return err
	}
	if len(raw) > maxShopifyAnswer {
		return errors.New("Shopify's answer is too large.")
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return &authError{"Shopify refused the access token."}
	case res.StatusCode == http.StatusForbidden:
		return &authError{"Shopify refused: the app needs the read_customers and read_orders scopes."}
	case res.StatusCode == http.StatusPaymentRequired || res.StatusCode == http.StatusLocked:
		return errors.New("The shop is unavailable (frozen or closed).")
	case res.StatusCode == http.StatusTooManyRequests:
		return errors.New("Shopify is busy. Try again in a moment.")
	case res.StatusCode >= 300:
		return fmt.Errorf("Shopify answered %d.", res.StatusCode)
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return errors.New("Shopify's answer is not JSON.")
	}
	if errs := blocking(env.Errors, env.Data); len(errs) > 0 {
		e := errs[0]
		if e.Extensions.Code == "ACCESS_DENIED" {
			return &authError{"Shopify refused: " + clip(e.Message, 200)}
		}
		if e.Extensions.Code == "THROTTLED" {
			return errors.New("Shopify is busy. Try again in a moment.")
		}
		return errors.New("Shopify: " + clip(e.Message, 200))
	}
	return json.Unmarshal(env.Data, out)
}

type gqlError struct {
	Message    string `json:"message"`
	Path       []any  `json:"path"`
	Extensions struct {
		Code string `json:"code"`
	} `json:"extensions"`
}

// optionalFields are fields an app may not be allowed to read: Shopify answers null with an
// error for each (an app without access to customer addresses, for shippingAddress), and the
// rest of the answer stands.
var optionalFields = map[string]bool{"shippingAddress": true}

// blocking is the errors that fail an answer: all of them without data, else those outside
// the optional fields.
func blocking(errs []gqlError, data json.RawMessage) []gqlError {
	if len(errs) == 0 || len(data) == 0 || string(data) == "null" {
		return errs
	}
	out := []gqlError{}
	for _, e := range errs {
		optional := false
		for _, p := range e.Path {
			if f, ok := p.(string); ok && optionalFields[f] {
				optional = true
			}
		}
		if !optional {
			out = append(out, e)
		}
	}
	return out
}

// checkShopify reads the shop's name with the credentials, so a wrong shop, app or token
// (or an app without the scopes) is caught when it is saved.
func (s *Service) checkShopify(ctx context.Context, shop string, c shopifyCred) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	token := c.secret
	if c.clientID != "" {
		tok, err := s.requestAppToken(ctx, shop, c)
		if err != nil {
			return "", err
		}
		if missing := missingScopes(tok.scope); len(missing) > 0 {
			return "", fmt.Errorf("The app lacks %s: add it to the app's version in the Dev Dashboard, release it and approve it on the shop.",
				strings.Join(missing, " and "))
		}
		token = tok.token
	}
	var out struct {
		Shop struct {
			Name string `json:"name"`
		} `json:"shop"`
	}
	if err := s.graphql(ctx, shop, token, `query { shop { name } }`, nil, &out); err != nil {
		return "", err
	}
	return clip(strings.TrimSpace(out.Shop.Name), 64), nil
}

// customerQuery reads the contact's customer and last orders. Its requested cost is about 350
// points (5 orders of 10 lines and 5 fulfilments), well under Shopify's 1,000 per query.
const customerQuery = `query PanelCustomer($q: String!, $orders: Int!, $items: Int!, $tags: Boolean!) {
  customers(first: 1, query: $q) {
    nodes {
      id
      displayName
      defaultEmailAddress { emailAddress }
      numberOfOrders
      amountSpent { amount currencyCode }
      tags @include(if: $tags)
      orders(first: $orders, sortKey: CREATED_AT, reverse: true) {
        nodes {
          id
          name
          createdAt
          cancelledAt
          displayFinancialStatus
          displayFulfillmentStatus
          currentTotalPriceSet { shopMoney { amount currencyCode } }
          totalRefundedSet { shopMoney { amount currencyCode } }
          discountCodes
          shippingLine { title }
          shippingAddress { city countryCodeV2 }
          lineItems(first: $items) {
            nodes { title variantTitle quantity originalUnitPriceSet { shopMoney { amount currencyCode } } }
          }
          fulfillments(first: 5) { trackingInfo(first: 5) { company number url } }
        }
      }
    }
  }
}`

type shopMoney struct {
	ShopMoney struct {
		Amount       string `json:"amount"`
		CurrencyCode string `json:"currencyCode"`
	} `json:"shopMoney"`
}

type shopifyCustomer struct {
	ID                  string `json:"id"`
	DisplayName         string `json:"displayName"`
	DefaultEmailAddress *struct {
		EmailAddress string `json:"emailAddress"`
	} `json:"defaultEmailAddress"`
	NumberOfOrders json.Number `json:"numberOfOrders"`
	AmountSpent    struct {
		Amount       string `json:"amount"`
		CurrencyCode string `json:"currencyCode"`
	} `json:"amountSpent"`
	Tags   []string `json:"tags"`
	Orders struct {
		Nodes []struct {
			ID                       string     `json:"id"`
			Name                     string     `json:"name"`
			CreatedAt                time.Time  `json:"createdAt"`
			CancelledAt              *time.Time `json:"cancelledAt"`
			DisplayFinancialStatus   string     `json:"displayFinancialStatus"`
			DisplayFulfillmentStatus string     `json:"displayFulfillmentStatus"`
			CurrentTotalPriceSet     shopMoney  `json:"currentTotalPriceSet"`
			TotalRefundedSet         *shopMoney `json:"totalRefundedSet"`
			DiscountCodes            []string   `json:"discountCodes"`
			ShippingLine             *struct {
				Title string `json:"title"`
			} `json:"shippingLine"`
			ShippingAddress *struct {
				City          *string `json:"city"`
				CountryCodeV2 *string `json:"countryCodeV2"`
			} `json:"shippingAddress"`
			LineItems struct {
				Nodes []struct {
					Title                string    `json:"title"`
					VariantTitle         *string   `json:"variantTitle"`
					Quantity             int       `json:"quantity"`
					OriginalUnitPriceSet shopMoney `json:"originalUnitPriceSet"`
				} `json:"nodes"`
			} `json:"lineItems"`
			Fulfillments []struct {
				TrackingInfo []struct {
					Company *string `json:"company"`
					Number  *string `json:"number"`
					URL     *string `json:"url"`
				} `json:"trackingInfo"`
			} `json:"fulfillments"`
		} `json:"nodes"`
	} `json:"orders"`
}

// searchPhrase quotes an email for the customers search (an exact phrase).
func searchPhrase(email string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `email:"` + r.Replace(email) + `"`
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// readShopify fills d with the contact's customer and last orders.
func (s *Service) readShopify(ctx context.Context, x *row, sub subject, d *Data) error {
	token, fresh, err := s.shopifyToken(ctx, x, false)
	if err != nil {
		return err
	}
	var out struct {
		Customers struct {
			Nodes []shopifyCustomer `json:"nodes"`
		} `json:"customers"`
	}
	vars := map[string]any{"q": searchPhrase(sub.Email), "orders": shopifyOrders, "items": shopifyItems, "tags": x.ShowTags}
	err = s.graphql(ctx, x.Target, token, customerQuery, vars, &out)
	if errors.Is(err, errAuth) && !fresh {
		// The app's token was revoked before its end: once more with a new one.
		if token, _, err = s.shopifyToken(ctx, x, true); err == nil {
			err = s.graphql(ctx, x.Target, token, customerQuery, vars, &out)
		}
	}
	if err != nil {
		return err
	}
	// The search is a phrase match; only the customer whose address is the contact's counts.
	var c *shopifyCustomer
	for i := range out.Customers.Nodes {
		n := &out.Customers.Nodes[i]
		if n.DefaultEmailAddress != nil && strings.EqualFold(strings.TrimSpace(n.DefaultEmailAddress.EmailAddress), sub.Email) {
			c = n
			break
		}
	}
	if c == nil {
		d.State = "empty"
		return nil
	}
	admin := adminURL(x.Target)
	count, _ := strconv.ParseInt(c.NumberOfOrders.String(), 10, 64)
	tags := []string{}
	if x.ShowTags && c.Tags != nil {
		tags = c.Tags
	}
	d.Customer = &Customer{Name: c.DisplayName, URL: admin + "/customers/" + url.PathEscape(gidNumber(c.ID)), OrdersCount: count,
		TotalSpent: c.AmountSpent.Amount, Currency: c.AmountSpent.CurrencyCode, Tags: tags}
	d.Orders = []Order{}
	for _, o := range c.Orders.Nodes {
		order := Order{ID: o.ID, Number: o.Name, URL: admin + "/orders/" + url.PathEscape(gidNumber(o.ID)), CreatedAt: o.CreatedAt,
			Total: o.CurrentTotalPriceSet.ShopMoney.Amount, Currency: o.CurrentTotalPriceSet.ShopMoney.CurrencyCode,
			FinancialStatus: strings.ToLower(o.DisplayFinancialStatus), FulfillmentStatus: strings.ToLower(o.DisplayFulfillmentStatus),
			Cancelled: o.CancelledAt != nil, CancelledAt: o.CancelledAt, DiscountCodes: []string{}, Items: []OrderItem{}, Tracking: []Tracking{}}
		if r := o.TotalRefundedSet; r != nil && positive(r.ShopMoney.Amount) {
			order.Refunded = r.ShopMoney.Amount
		}
		for _, code := range o.DiscountCodes {
			if code = strings.TrimSpace(code); code != "" {
				order.DiscountCodes = append(order.DiscountCodes, code)
			}
		}
		var ship Shipping
		if a := o.ShippingAddress; a != nil {
			ship.City, ship.Country = strings.TrimSpace(deref(a.City)), strings.ToUpper(strings.TrimSpace(deref(a.CountryCodeV2)))
		}
		if o.ShippingLine != nil {
			ship.Method = strings.TrimSpace(o.ShippingLine.Title)
		}
		if ship != (Shipping{}) {
			order.Shipping = &ship
		}
		for _, li := range o.LineItems.Nodes {
			order.Items = append(order.Items, OrderItem{Title: li.Title, Variant: deref(li.VariantTitle), Quantity: li.Quantity,
				Price: li.OriginalUnitPriceSet.ShopMoney.Amount})
		}
		for _, f := range o.Fulfillments {
			for _, ti := range f.TrackingInfo {
				tr := Tracking{Company: deref(ti.Company), Number: deref(ti.Number)}
				if u := deref(ti.URL); strings.HasPrefix(u, "https://") {
					tr.URL = u
				}
				if tr.Company != "" || tr.Number != "" || tr.URL != "" {
					order.Tracking = append(order.Tracking, tr)
				}
			}
		}
		d.Orders = append(d.Orders, order)
	}
	d.State = "ok"
	return nil
}
