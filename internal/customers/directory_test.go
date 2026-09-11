package customers_test

import (
	"testing"

	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/customers"
)

func TestUpsertGetList(t *testing.T) {
	d := customers.New()
	d.Upsert(customers.Customer{ID: "acme", Status: customers.StatusActive})
	d.Upsert(customers.Customer{ID: "broke-co", Status: customers.StatusSuspended})

	c, ok := d.Get("acme")
	if !ok || c.Status != customers.StatusActive {
		t.Fatalf("get acme: %+v ok=%v", c, ok)
	}
	list := d.List()
	if len(list) != 2 {
		t.Fatalf("list len %d", len(list))
	}
	if list[0].ID != "acme" || list[1].ID != "broke-co" {
		t.Fatalf("order: %+v", list)
	}
}

func TestSetStatus(t *testing.T) {
	d := customers.New()
	d.Upsert(customers.Customer{ID: "acme", Status: customers.StatusActive})
	if !d.SetStatus("acme", customers.StatusSuspended) {
		t.Fatal("set status failed")
	}
	c, _ := d.Get("acme")
	if c.Status != customers.StatusSuspended {
		t.Fatalf("status %q", c.Status)
	}
	if d.SetStatus("missing", customers.StatusActive) {
		t.Fatal("expected false for missing")
	}
}

func TestControlAllowedAndPatch(t *testing.T) {
	feats := customers.DefaultControlFeatures()
	if !customers.ControlAllowed(feats, customers.FeatureModerate) {
		t.Fatal("expected moderate allowed")
	}
	feats[customers.FeatureModerate] = false
	if customers.ControlAllowed(feats, customers.FeatureModerate) {
		t.Fatal("expected moderate denied")
	}
	feats[customers.FeatureControl] = false
	feats[customers.FeatureModerate] = true
	if customers.ControlAllowed(feats, customers.FeatureModerate) {
		t.Fatal("expected control master deny")
	}

	d := customers.New()
	d.Upsert(customers.Customer{ID: "acme", Status: customers.StatusActive, Features: customers.DefaultControlFeatures()})
	susp := customers.StatusSuspended
	c, ok := d.Patch("acme", &susp, map[string]any{customers.FeatureInject: false})
	if !ok || c.Status != customers.StatusSuspended {
		t.Fatalf("patch: %+v ok=%v", c, ok)
	}
	if customers.FeatureEnabled(c.Features, customers.FeatureInject) {
		t.Fatal("inject should be false")
	}
	if !customers.FeatureEnabled(c.Features, customers.FeatureModerate) {
		t.Fatal("moderate should remain true")
	}
}
