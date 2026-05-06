package openstack

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
)

// Client queries OpenStack Nova API and exposes VM metadata
// as a flat key/value store compatible with confd.
//
// Key structure produced:
//   /vms/<vm-name>/id                  = "uuid"
//   /vms/<vm-name>/status              = "ACTIVE"
//   /vms/<vm-name>/az                  = "nova"
//   /vms/<vm-name>/ip                  = "10.0.0.12"
//   /vms/<vm-name>/networks/<net>/ip   = "10.0.0.12"
//   /vms/<vm-name>/meta/<key>          = "<value>"

type Client struct {
	compute      *gophercloud.ServiceClient
	prefix       string
	pollInterval time.Duration
	filter       map[string]string
	lastPollTime time.Time
	cache        map[string]string
}

func New(authURL, credID, credSecret, projectID, region,
prefix string, filter map[string]string, pollInterval time.Duration) (*Client, error){
	opts := gophercloud.AuthOptions{
		IdentityEndpoint : authURL,
		ApplicationCredentialID: credID,
		ApplicationCredentialSecret: credSecret,
	}
	provider, err := openstack.AuthenticatedClient(context.Background(), opts)
	if err != nil{
		return nil, fmt.Errorf("Keystone auth failed : %w", err)
	}
	compute, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region : region, 
	})
	if err != nil{
		return nil, fmt.Errorf("nova client init failed : %w",err)
	}
	log.Printf("[openstack] authenticated — project=%s region=%s prefix=%s", projectID, region, prefix)
	
	return &Client{
		compute: compute,
		prefix: prefix,
		pollInterval: pollInterval,
		filter: filter,
		lastPollTime: time.Now().Add(-time.Minute),
		cache: make(map[string]string),
	}, nil

}

// GetValues fetches all VM metadata from Nova and returns matching key/value pairs.
func (c *Client) GetValues(keys []string) (map[string]string, error) {
	store, err := c.buildKeyValueStore()
	if err != nil {
		return nil, err
	}
	return filterByKeys(store, keys), nil
}
// WatchPrefix polls Nova using "changes-since" to detect VM changes.
// Blocks until a change is detected or stopChan is closed.
func (c *Client) WatchPrefix(prefix string, keys []string, waitIndex uint64, stopChan chan bool) (uint64, error) {
	log.Printf("[openstack] watching for changes (poll every %s)...", c.pollInterval)
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stopChan:
			return waitIndex, nil
		case <-ticker.C:
			changed, err := c.hasChangedSince(c.lastPollTime)
			if err != nil {
				log.Printf("[openstack] watch error: %v", err)
				continue
			}
			c.lastPollTime = time.Now()
			if changed {
				log.Printf("[openstack] change detected!")
				return waitIndex + 1, nil
			}
		}
	}
}
// HealthCheck verifies that the Nova API is reachable.
func (c *Client) HealthCheck() error {
	if c.compute == nil {
		return fmt.Errorf("compute client not initialized")
	}
	// Lightweight call: just list 1 server
	opts := servers.ListOpts{Limit: 1}
	_, err := servers.List(c.compute, opts).AllPages(context.Background())
	return err
}
// Close releases resources.
func (c *Client) Close() error {
	c.compute = nil
	return nil
}
// ─── Internal ────────────────────────────────────────────────────────────────
// buildKeyValueStore fetches all servers and builds the full confd key/value map.
func (c *Client) buildKeyValueStore() (map[string]string, error) {
	ctx := context.Background()
	opts := servers.ListOpts{AllTenants: false}
	allPages, err := servers.List(c.compute, opts).AllPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("nova list servers: %w", err)
	}
	allServers, err := servers.ExtractServers(allPages)
	if err != nil {
		return nil, fmt.Errorf("extract servers: %w", err)
	}
	result := make(map[string]string)
	for _, srv := range allServers {
		if !c.matchesFilter(srv) {
			continue
		}
		for k, v := range c.serverToKeys(srv) {
			result[k] = v
		}
	}
	log.Printf("[openstack] fetched %d VMs → %d keys", len(allServers), len(result))
	return result, nil
}
// hasChangedSince uses the Nova "changes-since" filter for efficient polling.
func (c *Client) hasChangedSince(since time.Time) (bool, error) {
	ctx := context.Background()
	opts := servers.ListOpts{
		ChangesSince: since.UTC().Format(time.RFC3339),
	}
	allPages, err := servers.List(c.compute, opts).AllPages(ctx)
	if err != nil {
		return false, err
	}
	changed, err := servers.ExtractServers(allPages)
	if err != nil {
		return false, err
	}
	return len(changed) > 0, nil
}
// serverToKeys converts a Nova server object into confd key/value pairs.
func (c *Client) serverToKeys(srv servers.Server) map[string]string {
	result := make(map[string]string)
	base := fmt.Sprintf("%s/%s", c.prefix, srv.Name)
	// Built-in Nova fields
	result[base+"/id"]     = srv.ID
	result[base+"/status"] = srv.Status
	result[base+"/az"]     = srv.AvailabilityZone
	// Extract IP addresses per network
	for network, addrs := range srv.Addresses {
		addrList, ok := addrs.([]interface{})
		if !ok || len(addrList) == 0 {
			continue
		}
		if addrMap, ok := addrList[0].(map[string]interface{}); ok {
			if ip, ok := addrMap["addr"].(string); ok {
				result[fmt.Sprintf("%s/networks/%s/ip", base, network)] = ip
				if _, exists := result[base+"/ip"]; !exists {
					result[base+"/ip"] = ip // first network = canonical IP
				}
			}
		}
	}
	// Custom metadata: openstack server set --property key=value
	for k, v := range srv.Metadata {
		result[fmt.Sprintf("%s/meta/%s", base, k)] = v
	}
	return result
}
// matchesFilter returns true if server metadata matches all filter key=value pairs.
func (c *Client) matchesFilter(srv servers.Server) bool {
	for k, v := range c.filter {
		if srv.Metadata[k] != v {
			return false
		}
	}
	return true
}
// filterByKeys returns only entries whose keys match the requested prefixes.
func filterByKeys(store map[string]string, keys []string) map[string]string {
	if len(keys) == 0 {
		return store
	}
	result := make(map[string]string)
	for k, v := range store {
		for _, prefix := range keys {
			if strings.HasPrefix(k, prefix) {
				result[k] = v
				break
			}
		}
	}
	return result
}