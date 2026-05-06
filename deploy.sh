#!/bin/bash
set -e

# Met à jour les IPs selon le Supfile
IPS=("157.136.255.84" "157.136.252.33" "157.136.253.210")

echo "🚀 Préparation des VMs..."
for IP in "${IPS[@]}"; do
  echo "→ Configuration de $IP"
  
  # 1. Installation de Nginx
  ssh -o StrictHostKeyChecking=no ubuntu@$IP "sudo apt-get update && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y nginx && sudo mkdir -p /etc/confd/conf.d /etc/confd/templates"
  
  # 2. Copie des binaires
  scp -o StrictHostKeyChecking=no ../confd/bin/confd-linux-amd64 ubuntu@$IP:/tmp/confd
  scp -o StrictHostKeyChecking=no bin/confd-plugin-openstack-linux-amd64 ubuntu@$IP:/tmp/confd-plugin-openstack
  
  # 3. Copie des templates de configuration
  scp -o StrictHostKeyChecking=no confd/conf.d/*.toml ubuntu@$IP:/tmp/
  scp -o StrictHostKeyChecking=no confd/templates/*.tmpl ubuntu@$IP:/tmp/
  
  # 4. Placement des fichiers de config et variables d'environnement
  echo "OS_AUTH_URL=$OS_AUTH_URL
OS_APPLICATION_CREDENTIAL_ID=$OS_APPLICATION_CREDENTIAL_ID
OS_APPLICATION_CREDENTIAL_SECRET=$OS_APPLICATION_CREDENTIAL_SECRET
OS_PROJECT_ID=$OS_PROJECT_ID
OS_REGION_NAME=$OS_REGION_NAME
OS_CONFD_FILTER=$OS_CONFD_FILTER" > /tmp/confd.env
  scp -o StrictHostKeyChecking=no /tmp/confd.env ubuntu@$IP:/tmp/confd.env
  
  ssh -o StrictHostKeyChecking=no ubuntu@$IP "sudo cp /tmp/*.toml /etc/confd/conf.d/ && sudo cp /tmp/*.tmpl /etc/confd/templates/ && sudo mv /tmp/confd.env /etc/confd/confd.env && sudo chown -R root:root /etc/confd"
  
  echo "  ✅ Terminé pour $IP"
done

echo "🚀 Lancement de sup full-deploy..."
sup -f sup/Supfile.hcl --parser sup-hcl2-parser openstack-cluster full-deploy
