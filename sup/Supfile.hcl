version = "0.5"

# Environment variables passed to all remote commands.
# Export OS_* vars locally before running sup.
env {
  PLUGIN_PATH   = "/usr/local/bin/confd-plugin-openstack"
  CONFD_CONFDIR = "/etc/confd"
  CONFD_BIN     = "/usr/local/bin/confd"
}

# ── Networks ─────────────────────────────────────────────────────────────────
# Update IPs after: openstack server list --format value -c Networks

network "openstack-cluster" {
  hosts = [
    "ubuntu@157.136.255.84",   # confd-web-01 (role=web)
    "ubuntu@157.136.252.33",   # confd-web-02 (role=web)
    "ubuntu@157.136.253.210",  # confd-db-01  (role=db)
  ]
}

network "web-only" {
  hosts = [
    "ubuntu@157.136.255.84",   # confd-web-01
    "ubuntu@157.136.252.33",   # confd-web-02
  ]
}

# ── Commands ─────────────────────────────────────────────────────────────────

command "install-confd" {
  desc = "Install confd and the OpenStack plugin binary"
  run  = "sudo install -m 755 /tmp/confd $${CONFD_BIN} && sudo install -m 755 /tmp/confd-plugin-openstack $${PLUGIN_PATH} && echo 'installed'"
}

command "push-confd-config" {
  desc = "Create confd directories on remote VMs"
  run  = "sudo mkdir -p /etc/confd/conf.d /etc/confd/templates && echo 'dirs ready'"
}

command "run-confd-onetime" {
  run  = "sudo bash -c 'export $(grep -v ^# /etc/confd/confd.env | xargs) && /usr/local/bin/confd plugin --plugin-path /usr/local/bin/confd-plugin-openstack --confdir /etc/confd --onetime' && echo 'confd done'"
}

command "start-confd-watch" {
  run  = "sudo bash -c 'export $(grep -v ^# /etc/confd/confd.env | xargs) && nohup /usr/local/bin/confd plugin --plugin-path /usr/local/bin/confd-plugin-openstack --confdir /etc/confd --watch > /tmp/confd.log 2>&1 &' && echo \"confd started\""
}

command "stop-confd" {
  desc = "Stop confd on all VMs"
  run  = "sudo pkill confd || true && echo 'stopped'"
}

command "show-config" {
  desc = "Print the generated nginx upstream config"
  run  = "cat /etc/nginx/conf.d/upstream.conf"
}

command "show-logs" {
  desc = "Tail the last 20 confd log lines"
  run  = "sudo tail -20 /var/log/confd.log"
}

# ── Targets ───────────────────────────────────────────────────────────────────

target "full-deploy" {
  commands = [
    "install-confd",
    "push-confd-config",
    "run-confd-onetime",
    "show-config",
  ]
}

target "start-watch" {
  commands = ["start-confd-watch"]
}

target "status" {
  commands = ["show-config", "show-logs"]
}

target "redeploy" {
  commands = ["stop-confd", "run-confd-onetime", "start-confd-watch"]
}
