package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/useopendesk/opendesk/internal/products"
)

func TestWrapFeatureDisabled(t *testing.T) {
	t.Parallel()
	for _, product := range []string{products.Mail, products.Tickets} {
		t.Run(product, func(t *testing.T) {
			cause := &products.DisabledError{Product: product, Reason: products.WorkspaceDisabled}
			var got *Error
			if !errors.As(Wrap(fmt.Errorf("operation: %w", cause)), &got) {
				t.Fatal("missing typed HTTP error")
			}
			if got.Status != 403 || got.Code != "feature_disabled" || got.Product != product || got.Reason != products.WorkspaceDisabled {
				t.Fatalf("error = %+v", got)
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatal(err)
			}
			if body["product"] != product || body["reason"] != products.WorkspaceDisabled {
				t.Fatalf("body = %s", raw)
			}
		})
	}
}
