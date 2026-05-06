# Démo : OpenStack Metadata + Confd (Event-Driven)

Ce document décrit comment déployer un cluster complet (2 serveurs Web Nginx + 1 serveur de Base de Données) qui s'auto-configure de manière **100% dynamique** en se basant *uniquement* sur les métadonnées des instances OpenStack (via notre plugin maison). 

Aucune base de données externe n'est requise. Les VMs sont la source de vérité.

---

## 🛠️ Étape 1 : Préparer l'environnement local

1. **Compiler les binaires** pour l'architecture des serveurs (Linux AMD64) :
   ```bash
   cd ~/Documents/confd
   GOOS=linux GOARCH=amd64 go build -buildvcs=false -o bin/confd-linux-amd64 ./cmd/confd
   
   cd ~/Documents/confd-plugin-openstack
   GOOS=linux GOARCH=amd64 go build -buildvcs=false -o bin/confd-plugin-openstack-linux-amd64 ./cmd/confd-plugin-openstack
   ```

2. **Créer une clé API OpenStack (Application Credential)** pour que notre plugin puisse se connecter à l'API :
   ```bash
   openstack --os-cloud openstack application credential create confd-demo-key \
     --description "Demo confd" \
     -f value -c id -c secret
   ```

3. **Exporter vos identifiants** dans le terminal (⚠️ attention à bien copier le TOUT début du secret, y compris s'il y a un tiret `-` !) :
   ```bash
   export OS_AUTH_URL="https://keystone.lal.in2p3.fr:5000/v3"
   export OS_APPLICATION_CREDENTIAL_ID="<TON_ID>"
   export OS_APPLICATION_CREDENTIAL_SECRET="<TON_SECRET_COMPLET>"
   export OS_PROJECT_ID="36a0bb6c348f484c971352f876cd4a33"
   export OS_REGION_NAME="lal"
   export OS_CONFD_FILTER="environment=demo"
   ```

---

## ☁️ Étape 2 : Créer le cluster OpenStack

Nous allons générer une paire de clés SSH sur OpenStack correspondant à votre clé locale `id_ed25519` afin d'éviter tout problème de `Permission denied`.

1. **Créer la clé sur OpenStack :**
   ```bash
   openstack --os-cloud openstack keypair create --public-key ~/.ssh/id_ed25519.pub local-mac-key || true
   ```

2. **Lancer les 3 VMs (2 Web, 1 DB)** avec les bonnes métadonnées (`role` et `app_port`) :
   ```bash
   openstack --os-cloud openstack server create --flavor m1.small --image "ubuntu-2404.amd64-genericcloud.20260108" --key-name local-mac-key --network private --property role=web --property app_port=8080 --property environment=demo --wait confd-web-01
   
   openstack --os-cloud openstack server create --flavor m1.small --image "ubuntu-2404.amd64-genericcloud.20260108" --key-name local-mac-key --network private --property role=web --property app_port=8080 --property environment=demo --wait confd-web-02
   
   openstack --os-cloud openstack server create --flavor m1.small --image "ubuntu-2404.amd64-genericcloud.20260108" --key-name local-mac-key --network private --property role=db --property app_port=5432 --property environment=demo --wait confd-db-01
   ```

3. **Attacher des IP Flottantes :**
   *(Note : Assure-toi d'avoir 3 IPs libres via `openstack floating ip list`)*
   ```bash
   openstack --os-cloud openstack server add floating ip confd-web-01 <IP_1>
   openstack --os-cloud openstack server add floating ip confd-web-02 <IP_2>
   openstack --os-cloud openstack server add floating ip confd-db-01 <IP_3>
   ```

4. **Nettoyer les Known Hosts (Optionnel mais recommandé) :**
   Afin d'éviter le message "Host identification has changed", lance :
   ```bash
   ssh-keygen -R 157.136.255.84
   ssh-keygen -R <IP_2>
   ssh-keygen -R <IP_3>
   ```

---

## 🚀 Étape 3 : Déploiement automatisé

1. **Mettre à jour vos scripts :**
   - Édite le fichier `sup/Supfile.hcl` pour y mettre tes IPs.
   - Édite le script `deploy.sh` pour y mettre la liste des mêmes IPs.

2. **Charger la clé SSH en mémoire :**
   ```bash
   eval $(ssh-agent -s)
   ssh-add ~/.ssh/id_ed25519
   ```

3. **Lancer le déploiement complet :**
   Le script va injecter de manière sécurisée vos `OS_*` (via un fichier `/etc/confd/confd.env`), installer les dépendances (Nginx), et lancer `sup full-deploy`.
   ```bash
   ./deploy.sh
   ```

---

## 👀 Étape 4 : Visualiser la magie (Event-Driven Watch)

### 1. Démarrer le processus d'écoute
Lance la commande `start-watch` de Sup. Celle-ci va faire tourner `confd` en tâche de fond (`nohup`) sur les VMs. `confd` va interroger l'API OpenStack toutes les 15s.
```bash
sup -f sup/Supfile.hcl --parser sup-hcl2-parser openstack-cluster start-watch
```

### 2. Voir l'état initial des fichiers générés
```bash
# Vérifier la configuration du Load Balancer Web (confd-web-01)
ssh -o StrictHostKeyChecking=no ubuntu@157.136.255.84 "cat /etc/nginx/conf.d/upstream.conf"

# Vérifier la configuration de l'App Base de Données (confd-db-01)
ssh -o StrictHostKeyChecking=no ubuntu@157.136.253.210 "cat /etc/app.conf"
```

### 3. Modifier l'infrastructure "à chaud" !
Changeons les ports de l'application directement dans les métadonnées OpenStack :

```bash
openstack --os-cloud openstack server set --property app_port=9090 confd-web-01
openstack --os-cloud openstack server set --property app_port=6432 confd-db-01
```

### 4. Suivre la mise à jour dynamique
Affiche les logs de `confd` sur l'une des machines pour voir la mise à jour s'opérer (attends maximum 15 secondes) :
```bash
ssh -o StrictHostKeyChecking=no ubuntu@157.136.255.84 "sudo tail -f /tmp/confd.log"
```

*Quand tu vois "Target config out of sync", cela veut dire que `confd` a ré-écrit le fichier !*

Vérifie que les ports ont bien été modifiés dans le fichier de configuration distant :
```bash
ssh -o StrictHostKeyChecking=no ubuntu@157.136.255.84 "cat /etc/nginx/conf.d/upstream.conf"
```
*(Tu devrais voir `9090` pour `confd-web-01` et une date `Last updated` actualisée !)*

---

## 🌟 Étape 5 : Démonstrations Avancées (Wow Effect)

Pour une démonstration visuelle percutante, ouvre **3 terminaux côte à côte** sur ton écran :

### 👁️ Terminal 1 : Monitoring du Load Balancer (Nginx)
Ce terminal va rafraîchir le fichier Nginx toutes les secondes pour voir les changements en direct.
```bash
ssh -t -o StrictHostKeyChecking=no ubuntu@157.136.255.84 "watch -n 1 cat /etc/nginx/conf.d/upstream.conf"
```

### 👁️ Terminal 2 : Monitoring de la Database (App)
Ce terminal surveille l'application connectée à la base de données.
```bash
ssh -t -o StrictHostKeyChecking=no ubuntu@157.136.253.210 "watch -n 1 cat /etc/app.conf"
```

### 🛠️ Terminal 3 : Ton panneau de contrôle (Local)
C'est ici que tu vas lancer les événements OpenStack pour voir les Terminaux 1 et 2 réagir magiquement.

#### Démo A : Scale-out automatique (Ajout d'une 3ème VM Web)
Imagine que tu as un pic de trafic. Lance une nouvelle VM Web :
```bash
openstack --os-cloud openstack server create --flavor m1.small --image "ubuntu-2404.amd64-genericcloud.20260108" --key-name local-mac-key --network private --property role=web --property app_port=8080 --property environment=demo --wait confd-web-03
```
👉 *Regarde le **Terminal 1** : Dans les 15 secondes, la nouvelle adresse IP va s'ajouter toute seule dans le bloc `upstream backend_web` de Nginx !*

#### Démo B : Mode Maintenance (Failover)
Le serveur `confd-web-01` doit être mis à jour. On le retire du Load Balancer en changeant son rôle :
```bash
openstack --os-cloud openstack server set --property role=maintenance confd-web-01
```
👉 *Regarde le **Terminal 1** : Nginx retire instantanément `confd-web-01` de la rotation !*

*(Pour le remettre en ligne :)*
```bash
openstack --os-cloud openstack server set --property role=web confd-web-01
```

#### Démo C : Migration de Base de données
Ta DB principale a cramé, tu as basculé sur un failover qui écoute sur le port `3306`. Mettons à jour le port :
```bash
openstack --os-cloud openstack server set --property app_port=3306 confd-db-01
```
👉 *Regarde le **Terminal 2** : Le paramètre `endpoint` se met à jour en `172.16.10.x:3306` et l'application se recharge !*

---

## 🧹 Étape 6 : Nettoyage (Clean Up)

Une fois tes démos terminées, efface les traces pour libérer les ressources.

1. **Stopper le processus confd sur tout le cluster :**
   *(Pour arrêter le background loop qu'on a lancé à l'étape 4)*
   ```bash
   sup -f sup/Supfile.hcl --parser sup-hcl2-parser openstack-cluster stop-confd
   ```

2. **Détruire TOUTES les VMs créées :**
   ```bash
   openstack --os-cloud openstack server delete confd-web-01 confd-web-02 confd-web-03 confd-db-01 --wait
   ```

3. **Supprimer la clé API OpenStack (Optionnel) :**
   ```bash
   openstack --os-cloud openstack application credential delete confd-demo-key
   ```

🎉 **La boucle est bouclée, tu as un système d'orchestration 100% dynamique !**
