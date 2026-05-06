# confd-plugin-openstack

An OpenStack backend plugin for [confd](https://github.com/abtreece/confd) that enables dynamic, event-driven configuration management using OpenStack VM metadata as a source of truth.

## Overview

Traditional configuration management systems often rely on external databases (like etcd, Consul, or Redis) to store infrastructure state. This plugin eliminates the need for an external key-value store by directly utilizing OpenStack Nova metadata.

By assigning custom properties to Virtual Machines (e.g., `role=web`, `app_port=8080`), `confd-plugin-openstack` continuously polls the OpenStack API for changes. When a VM is created, deleted, or modified, the plugin constructs a virtual key-value tree and seamlessly triggers `confd` to regenerate configuration files and reload services (such as NGINX or HAProxy) in near real-time.

## Architecture

The system operates on an event-driven polling mechanism leveraging Gophercloud. 

1. **Polling**: The plugin polls the Nova API (optimizing with the `changes-since` filter).
2. **Key-Value Construction**: OpenStack instances matching a specific filter are translated into a hierarchical key-value structure.
3. **Template Evaluation**: `confd` evaluates Go templates using the generated keys.
4. **Service Reload**: If the generated configuration differs from the current configuration, `confd` performs an atomic write and triggers the configured reload command.

### Key Structure

The plugin generates a virtual hierarchy accessible by `confd` templates:

```text
/vms/<vm-name>/id                  = "uuid"
/vms/<vm-name>/status              = "ACTIVE"
/vms/<vm-name>/az                  = "nova"
/vms/<vm-name>/ip                  = "172.16.10.45"
/vms/<vm-name>/networks/<net>/ip   = "172.16.10.45"
/vms/<vm-name>/meta/<key>          = "<value>"
```

## Prerequisites

- Go 1.22 or higher
- An OpenStack environment with Keystone Application Credentials
- A compiled binary of `confd` supporting the plugin interface

## Building the Plugin

The plugin is compiled as a standalone binary that communicates with `confd` over gRPC.

```bash
make build
```

This will output the binary to `bin/confd-plugin-openstack`.

## Configuration

The plugin requires OpenStack credentials to communicate with the Nova API. These should be provided as environment variables.

| Variable | Description |
|----------|-------------|
| `OS_AUTH_URL` | Keystone Identity Endpoint URL |
| `OS_APPLICATION_CREDENTIAL_ID` | OpenStack Application Credential ID |
| `OS_APPLICATION_CREDENTIAL_SECRET`| OpenStack Application Credential Secret |
| `OS_PROJECT_ID` | Project / Tenant ID |
| `OS_REGION_NAME` | OpenStack Region (e.g., `RegionOne`) |
| `OS_CONFD_PREFIX` | Prefix for the generated keys (default: `/vms`) |
| `OS_CONFD_FILTER` | Metadata filter to restrict tracked VMs (e.g., `environment=demo`) |

## Usage

Ensure the plugin binary is executable and accessible.

### Running with confd (One-Time Mode)

To generate configurations once and exit:

```bash
confd plugin \
  --plugin-path "/path/to/confd-plugin-openstack" \
  --confdir "/etc/confd" \
  --onetime
```

### Running with confd (Watch Mode)

To run `confd` as a daemon that continuously watches for OpenStack metadata changes:

```bash
confd plugin \
  --plugin-path "/path/to/confd-plugin-openstack" \
  --confdir "/etc/confd" \
  --watch
```

## Template Example

The following is an example `nginx.conf.tmpl` demonstrating how to iterate over instances and filter them based on their OpenStack metadata `role`.

```go
upstream backend {
{{- range $name := ls "/vms"}}
{{- $role := getv (printf "/vms/%s/meta/role" $name) ""}}
{{- if eq $role "web"}}
{{- $ip   := getv (printf "/vms/%s/ip" $name) ""}}
{{- $port := getv (printf "/vms/%s/meta/app_port" $name) "8080"}}
    server {{$ip}}:{{$port}};  # {{$name}}
{{- end}}
{{- end}}
}
```

## Automation and Deployment

A deployment script (`deploy.sh`) and an HCL configuration (`sup/Supfile.hcl`) are provided to automate the distribution of the plugin, `confd` templates, and environment variables across a cluster of virtual machines.

To initiate a full deployment using `sup`:

```bash
sup -f sup/Supfile.hcl --parser sup-hcl2-parser openstack-cluster full-deploy
```

## License

This project is licensed under the MIT License.
