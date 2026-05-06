package main

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/abtreece/confd/pkg/backends/plugin/api"
	"github.com/hashicorp/go-plugin"
	"github.com/maelanjais/confd-plugin-openstack/pkg/backends/openstack"
)

func main() {

	authURL    := envOr("OS_AUTH_URL", "")
	credID     := envOr("OS_APPLICATION_CREDENTIAL_ID", "")
	credSecret := envOr("OS_APPLICATION_CREDENTIAL_SECRET", "")
	projectID  := envOr("OS_PROJECT_ID", "")
	region     := envOr("OS_REGION_NAME", "RegionOne")
	prefix     := envOr("OS_CONFD_PREFIX", "/vms")
	filter     := parseFilter(envOr("OS_CONFD_FILTER", ""))
	pollSecs   := 15

	backend, err := openstack.New(
		authURL,credID,credSecret,projectID,region,prefix,filter, time.Duration(pollSecs)*time.Second,
	)
	if err != nil {
		log.Fatalf("[confd-plugin-openstack] failed to initialize: %v", err)
	}
	log.Printf("[confd-plugin-openstack] started - prefix = %s, poll =%ds", prefix, pollSecs)

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: api.Handshake,
		Plugins: map[string]plugin.Plugin{
			"backend": &api.ConfdBackendPlugin{
				Impl: &ProviderWrapper{client: backend},
			},
		},

	})
}

type ProviderWrapper struct {
	client *openstack.Client
}

func (w *ProviderWrapper) GetValues(keys []string) (map[string]string, error){
	return w.client.GetValues(keys)
}
func (w *ProviderWrapper) WatchPrefix(prefix string, keys []string, waitIndex uint64) (uint64, error){
	stopChan := make(chan bool)
	return w.client.WatchPrefix(prefix,keys,waitIndex,stopChan)
}

func (w *ProviderWrapper) HealthCheck() error { return w.client.HealthCheck() }
func (w *ProviderWrapper) Close() error        { return w.client.Close() }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def 
}

func parseFilter(s string) map[string]string{
	result := make(map[string]string)
	if s == ""{
		return result
	}
	for _, part := range strings.Split(s, ","){
		kv := strings.SplitN(part,"=", 2)
		if len(kv) == 2{
			result[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return result
}