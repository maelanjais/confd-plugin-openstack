package openstack

import (
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
)

// ── filterByKeys ─────────────────────────────────────────────────────────

func TestFilterByKeys_EmptyKeys(t *testing.T) {
	store := map[string]string{
		"/vms/web-01/ip":     "10.0.0.1",
		"/vms/web-01/status": "ACTIVE",
	}
	got := filterByKeys(store, []string{})
	if len(got) != 2 {
		t.Errorf("empty keys should return full store, got %d entries", len(got))
	}
}

func TestFilterByKeys_WithPrefix(t *testing.T) {
	store := map[string]string{
		"/vms/web-01/ip":        "10.0.0.1",
		"/vms/web-01/status":    "ACTIVE",
		"/vms/db-01/ip":         "10.0.0.9",
		"/vms/web-01/meta/role": "web",
	}
	got := filterByKeys(store, []string{"/vms/web-01"})
	if len(got) != 3 {
		t.Errorf("expected 3 keys for /vms/web-01, got %d", len(got))
	}
	if got["/vms/db-01/ip"] != "" {
		t.Error("db-01 should not be in result")
	}
}

func TestFilterByKeys_MultiplePrefix(t *testing.T) {
	store := map[string]string{
		"/vms/web-01/ip": "10.0.0.1",
		"/vms/db-01/ip":  "10.0.0.9",
		"/vms/lb-01/ip":  "10.0.0.5",
	}
	got := filterByKeys(store, []string{"/vms/web-01", "/vms/db-01"})
	if len(got) != 2 {
		t.Errorf("expected 2 keys, got %d", len(got))
	}
}

// ── matchesFilter ─────────────────────────────────────────────────────────

func TestMatchesFilter_EmptyFilter(t *testing.T) {
	c := &Client{filter: map[string]string{}}
	srv := fakeServer("web-01", map[string]string{"role": "web"})
	if !c.matchesFilter(srv) {
		t.Error("empty filter should match any server")
	}
}

func TestMatchesFilter_Match(t *testing.T) {
	c := &Client{filter: map[string]string{"role": "web", "env": "prod"}}
	srv := fakeServer("web-01", map[string]string{"role": "web", "env": "prod", "port": "8080"})
	if !c.matchesFilter(srv) {
		t.Error("should match server with role=web env=prod")
	}
}

func TestMatchesFilter_NoMatch(t *testing.T) {
	c := &Client{filter: map[string]string{"role": "web"}}
	srv := fakeServer("db-01", map[string]string{"role": "db"})
	if c.matchesFilter(srv) {
		t.Error("should NOT match server with role=db")
	}
}

func TestMatchesFilter_PartialMatch(t *testing.T) {
	c := &Client{filter: map[string]string{"role": "web", "env": "prod"}}
	srv := fakeServer("web-01", map[string]string{"role": "web"}) // missing env
	if c.matchesFilter(srv) {
		t.Error("should NOT match when a filter key is missing")
	}
}

// ── serverToKeys ─────────────────────────────────────────────────────────

func TestServerToKeys_BasicFields(t *testing.T) {
	c := &Client{prefix: "/vms"}
	srv := fakeServer("web-01", map[string]string{"role": "web", "port": "8080"})
	srv.ID     = "abc-123"
	srv.Status = "ACTIVE"

	keys := c.serverToKeys(srv)

	if keys["/vms/web-01/id"] != "abc-123" {
		t.Errorf("id key wrong: %s", keys["/vms/web-01/id"])
	}
	if keys["/vms/web-01/status"] != "ACTIVE" {
		t.Errorf("status key wrong: %s", keys["/vms/web-01/status"])
	}
	if keys["/vms/web-01/meta/role"] != "web" {
		t.Errorf("meta/role key wrong: %s", keys["/vms/web-01/meta/role"])
	}
	if keys["/vms/web-01/meta/port"] != "8080" {
		t.Errorf("meta/port key wrong: %s", keys["/vms/web-01/meta/port"])
	}
}

func TestServerToKeys_CustomPrefix(t *testing.T) {
	c := &Client{prefix: "/cluster/nodes"}
	srv := fakeServer("node-01", map[string]string{})

	keys := c.serverToKeys(srv)

	if _, ok := keys["/cluster/nodes/node-01/id"]; !ok {
		t.Error("custom prefix not applied")
	}
}

// ── Client struct defaults ────────────────────────────────────────────────

func TestClientDefaults(t *testing.T) {
	c := &Client{
		prefix:       "/vms",
		pollInterval: 15 * time.Second,
		filter:       map[string]string{},
		cache:        make(map[string]string),
	}
	if c.prefix != "/vms" {
		t.Errorf("prefix = %q", c.prefix)
	}
	if c.pollInterval != 15*time.Second {
		t.Errorf("pollInterval = %v", c.pollInterval)
	}
}

// ── HealthCheck without compute client ────────────────────────────────────

func TestHealthCheck_NotInitialized(t *testing.T) {
	c := &Client{compute: nil}
	err := c.HealthCheck()
	if err == nil {
		t.Error("HealthCheck should fail when compute is nil")
	}
}

func TestClose(t *testing.T) {
	c := &Client{compute: nil}
	if err := c.Close(); err != nil {
		t.Errorf("Close() should not error, got: %v", err)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────

// fakeServer builds a minimal servers.Server for testing.
// Uses a local struct to avoid importing gophercloud in tests.
type fakeServerStruct struct {
	Name             string
	ID               string
	Status           string
	AvailabilityZone string
	Metadata         map[string]string
	Addresses        map[string]interface{}
}

func fakeServer(name string, meta map[string]string) servers.Server {
	return servers.Server{
		Name:      name,
		Metadata:  meta,
		Addresses: map[string]interface{}{},
	}
}
