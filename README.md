# Deployment Runbook

## Cloud-Native Open Banking API Platform with Federated Governance

Using WSO2 API Manager, Kong Gateway, Envoy Gateway and Omni Gateway (MuleSoft)

| | |
|---|---|
| **Intern** | Mayess Boussaada |
| **Supervisor** | Imen Frigui |
| **Duration** | 2 months |

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [Prerequisites](#2-prerequisites)
3. [Kubernetes Cluster (K3d)](#3-kubernetes-cluster-k3d)
4. [Gateway API CRDs](#4-gateway-api-crds)
5. [TLS Certificate Management](#5-tls-certificate-management)
6. [MySQL Database](#6-mysql-database)
7. [Custom WSO2 APIM Image](#7-custom-wso2-apim-image)
8. [Envoy Gateway Controller](#8-envoy-gateway-controller)
9. [Platform Gateway Resource](#9-platform-gateway-resource)
10. [WSO2 API Manager — Control Plane](#10-wso2-api-manager--control-plane)
11. [WSO2 Traffic Manager](#11-wso2-traffic-manager)
12. [Data Plane — Universal Gateway](#12-data-plane--universal-gateway)
13. [Data Plane — Kong Gateway](#13-data-plane--kong-gateway)
14. [Data Plane — Envoy Gateway](#14-data-plane--envoy-gateway)
15. [Data Plane — MuleSoft Omni/Flex Gateway](#15-data-plane--mulesoft-omniflex-gateway)
16. [Observability — Moesif Integration](#16-observability--moesif-integration)
17. [Observability — On-Premise Analytics (ELK Stack)](#17-observability--on-premise-analytics-elk-stack)

---

## 1. Introduction

This runbook describes, step by step, the full deployment procedure for the cloud-native Open Banking API platform designed during the summer internship. The platform is composed of:

- A **Kubernetes cluster** provisioned with K3d inside WSL 2.
- A **Control Plane** based on WSO2 API Manager 4.7.0.
- Three **Data Plane gateways**: the WSO2 Universal Gateway, Kong Gateway, Envoy Gateway and Omni Gateway.

---

## 2. Prerequisites

- Windows with WSL 2 enabled (Ubuntu distribution installed).
- Docker Desktop running and integrated with WSL 2.
- `kubectl`, `helm` v3, `k3d`, and `curl` installed inside the WSL 2 environment.
- Docker Hub account (used for pushing custom images).
- The project repository cloned locally with all configuration files (`shared/`, `secrets/`, `database/`, `keystores/`, `control-plane/`, `traffic-manager/`, `gateways/`, `agents/`, `common-agent/`, `routing-controller/`).

---

## 3. Kubernetes Cluster (K3d)

### 3.1 Create the K3d Cluster

**Purpose:** Provision a lightweight K3s-based Kubernetes cluster using K3d. The cluster configuration disables the built-in Traefik ingress controller because the platform uses its own proxy and gateway components.

```bash
k3d cluster create --config shared/k3d-cluster-config.yaml
```

### 3.2 Start the Cluster

**Purpose:** Start the cluster if it was previously stopped.

```bash
k3d cluster start api-platform
```

**Verification:**

```bash
kubectl cluster-info
kubectl get nodes
```

---

## 4. Gateway API CRDs

**Purpose:** Install the Kubernetes Gateway API Custom Resource Definitions (experimental channel) which are required by Envoy Gateway, Kong Gateway, and the WSO2 Universal Gateway to define `GatewayClass`, `Gateway`, and `HTTPRoute` resources.

```bash
kubectl apply -f \
  https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml
```

**Verification:**

```bash
kubectl get crd | grep gateway
```

---

## 5. TLS Certificate Management

### 5.1 Install cert-manager

**Purpose:** Deploy cert-manager in its own namespace to automate the issuance and renewal of TLS certificates used by the platform components.

```bash
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  -f shared/certs/cert-manager-values.yaml
```

**Verification:**

```bash
kubectl get pods -n cert-manager
helm list -A
```

### 5.2 Create the Control Plane Namespace

**Purpose:** Create the `apim-cp` namespace that will host the WSO2 API Manager control plane and its associated resources (MySQL, TLS secrets, keystores).

```bash
kubectl create namespace apim-cp
```

### 5.3 Create the Cluster Issuer

**Purpose:** Deploy a self-signed `ClusterIssuer` that cert-manager will use to issue TLS certificates within the cluster.

```bash
kubectl apply -f shared/certs/issuer.yaml
```

**Verification:**

```bash
kubectl get clusterissuer
```

### 5.4 Create the Wildcard TLS Certificate

**Purpose:** Request a wildcard certificate (e.g. `*.wso2.com`) that will be used by the platform gateways and the control plane to serve HTTPS traffic.

```bash
kubectl apply -f shared/certs/wildcard-certificate.yaml
```

**Verification:**

```bash
kubectl get certificate -n apim-cp
kubectl get secret wso2-wildcard-tls -n apim-cp
```

---

## 6. MySQL Database

### 6.1 Install MySQL

**Purpose:** Deploy a MySQL instance inside the `apim-cp` namespace. WSO2 API Manager requires two databases (`apim_db` and `shared_db`) for storing API metadata, user information, and runtime data.

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

helm install mysql bitnami/mysql \
  --namespace apim-cp \
  -f secrets/mysql-values.yaml
```

**MySQL values file:**

```yaml
image:
  repository: bitnamilegacy/mysql
  tag: 8.4.3-debian-12-r0

auth:
  rootPassword: "root"
  database: apim_db
  username: apimadmin
  password: "root"

primary:
  persistence:
    enabled: true
    size: 5Gi
```

**Verification:**

```bash
kubectl get pods -n apim-cp
helm list -n apim-cp
```

### 6.2 Create Databases and Grant Permissions

**Purpose:** Create the two required databases and grant the application users the necessary privileges.

```bash
kubectl exec -it -n apim-cp mysql-0 -- mysql -uroot -p
```

Inside the MySQL shell:

```sql
CREATE DATABASE apim_db
  CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;

CREATE DATABASE shared_db
  CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;

GRANT ALL ON apim_db.*   TO 'apimadmin'@'%';
GRANT ALL ON shared_db.* TO 'sharedadmin'@'%';
FLUSH PRIVILEGES;
```

### 6.3 Import Database Schemas

**Purpose:** Copy the SQL schema files into the MySQL pod and execute them to create the required table structures for WSO2 API Manager.

```bash
# Copy SQL files into the pod
kubectl cp -c mysql database/apim_db.sql   apim-cp/mysql-0:/tmp/apim_db.sql
kubectl cp -c mysql database/shared_db.sql apim-cp/mysql-0:/tmp/shared_db.sql

# Import schemas
kubectl exec -n apim-cp -c mysql mysql-0 -- \
  bash -c 'mysql -uroot -proot shared_db < /tmp/shared_db.sql'

kubectl exec -n apim-cp -c mysql mysql-0 -- \
  bash -c 'mysql -uroot -proot apim_db < /tmp/apim_db.sql'
```

**Verification:**

```bash
kubectl exec -n apim-cp -c mysql mysql-0 -- \
  mysql -uroot -proot -e \
  "SELECT COUNT(*) AS table_count
   FROM information_schema.tables
   WHERE table_schema='apim_db';"

kubectl exec -n apim-cp -c mysql mysql-0 -- \
  mysql -uroot -proot -e \
  "SELECT COUNT(*) AS table_count
   FROM information_schema.tables
   WHERE table_schema='shared_db';"
```

---

## 7. Custom WSO2 APIM Image

**Purpose:** Build a custom Docker image of WSO2 API Manager that includes project-specific configurations (e.g. MySQL driver, deployment TOML overrides) and push it to Docker Hub so that Kubernetes can pull it.

```bash
docker build -t mayess03/wso2am-acp-mysql:4.7.0 ./wso2am-acp-build
docker push mayess03/wso2am-acp-mysql:4.7.0
```

**Verification:** The push command outputs the image digest (`sha256:...`) confirming a successful upload.

---

## 8. Envoy Gateway Controller

**Purpose:** Install the Envoy Gateway controller which watches Kubernetes Gateway API resources (`Gateway`, `HTTPRoute`) and translates them into Envoy Proxy configurations. It acts as the main ingress proxy for the platform and is also used by the Envoy data-plane gateway. The `enableBackend` flag activates the Backend API extension needed for cross-namespace routing.

```bash
helm install envoy-gateway \
  oci://docker.io/envoyproxy/gateway-helm \
  --version v1.7.0 \
  -n envoy-gateway-system \
  --create-namespace \
  --set config.envoyGateway.extensionApis.enableBackend=true \
  --set envoyGateway.gateway.experimentalFeatures.enabled=true
```

**Verification:**

```bash
kubectl get pods -n envoy-gateway-system
```

---

## 9. Platform Gateway Resource

**Purpose:** Create the Kubernetes `GatewayClass` and `Gateway` resources that define the platform-level entry point. This Gateway is managed by the Envoy Gateway controller and listens for incoming HTTPS traffic.

```bash
kubectl apply -f routing-controller/sample-gateway.yaml
```

**Verification:**

```bash
kubectl get gatewayclass
kubectl get gateway -n apim-cp
```

---

## 10. WSO2 API Manager — Control Plane

### 10.1 Create Keystore Secrets

**Purpose:** Store the WSO2 Java KeyStore files (`wso2carbon.jks` and `client-truststore.jks`) as Kubernetes secrets so the control plane pods can mount them securely.

```bash
kubectl create secret generic apim-keystore-secret \
  -n apim-cp \
  --from-file=wso2carbon.jks=./keystores/wso2carbon.jks \
  --from-file=client-truststore.jks=./keystores/client-truststore.jks
```

**Verification:**

```bash
kubectl get secret apim-keystore-secret -n apim-cp
```

### 10.2 Install the Control Plane

**Purpose:** Deploy the WSO2 API Manager control plane using the local Helm chart. The control plane provides the Publisher, DevPortal, and Admin interfaces for centralized API lifecycle management.

```bash
helm repo add wso2 https://helm.wso2.com
helm repo update

helm install apim-acp ./control-plane \
  -n apim-cp \
  -f control-plane/updated-values.yaml
```

**Verification:**

```bash
kubectl get pods -n apim-cp
helm list -n apim-cp
```

**Note:** The domain names must be added to the `/etc/hosts` file.

**Access Links:**

- **Publisher Portal:** https://am.wso2.com/publisher
- **Developer Portal:** https://am.wso2.com/devportal
- **Admin Portal:** https://am.wso2.com/admin

---

## 11. WSO2 Traffic Manager

**Purpose:** Deploy the WSO2 Traffic Manager component, which handles throttling, rate limiting, and event-based communication between the control plane and the Universal Gateway.

```bash
helm install apim-tm ./traffic-manager \
  -n apim-cp \
  -f traffic-manager/values-tm.yaml
```

**Verification:**

```bash
kubectl get pods -n apim-cp
```

---

## 12. Data Plane — Universal Gateway

**Purpose:** Deploy the WSO2 Universal Gateway, the native runtime gateway of WSO2 API Manager. APIs published through the Publisher portal are deployed directly to this gateway, which enforces the security policies and mediation logic defined in the control plane.

```bash
helm install apim-gw ./gateways/universal \
  -n apim-cp \
  -f ./gateways/universal/values-gw.yaml
```

Apply the HTTPRoute patch so the gateway is reachable through the platform ingress:

```bash
kubectl apply -f gateways/universal/gw-route-patch.yaml
```

**Verification:**

```bash
kubectl get pods -n apim-cp
kubectl get httproute -n apim-cp
```

### 12.1 Deploy the Test Backend

**Purpose:** Deploy a simple echo service inside the cluster that will serve as the backend endpoint for the API created in the Publisher portal. This allows end-to-end verification of the full request path: client → Universal Gateway → echo backend.

```bash
kubectl apply -f gateways/universal/echo.yaml
```

**Verification:**

```bash
kubectl get pods -l app=echo-backend
kubectl get svc echo-backend
```

### 12.2 Create the API in the Publisher Portal

**Purpose:** Use the WSO2 Publisher portal to define, configure, and publish an API that will be deployed to the Universal Gateway. This step validates the control-plane-to-data-plane integration.

1. Open the Publisher portal.
2. Click **Create API** → **REST API** → **Design New REST API**.
3. Fill in the API details:
   - **Name:** `EchoAPI`
   - **Context:** `/echo`
   - **Version:** `1.0.0`
   - **Endpoint:** the cluster-internal echo backend URL
4. Define the API resources (e.g. `GET /`).
5. Under **Runtime Configurations**, set the desired security scheme (OAuth2 / JWT).
6. Navigate to **Deployments**, select the Universal Gateway label, and deploy the API.
7. Change the lifecycle state to **Published**.

### 12.3 Test the API in the DevPortal

**Purpose:** Subscribe to the published API from the Developer Portal, generate an access token, and invoke the API to confirm that the Universal Gateway correctly routes and secures the traffic.

1. Open the DevPortal.
2. Locate the `EchoAPI` and click **Subscribe** using an existing application (or create a new one).
3. Under the application, go to **Sandbox Keys** and generate an OAuth2 access token.
4. Use the built-in **Try It** console or `curl` to invoke the API.

A successful response from the echo backend confirms that the full control-plane → Universal Gateway → backend path is operational.

---

## 13. Data Plane — Kong Gateway

### 13.1 Create the Kong Namespace

```bash
kubectl create namespace kong
```

### 13.2 Install Standard Gateway API CRDs

**Purpose:** Kong requires the standard (non-experimental) Gateway API CRDs in addition to the experimental ones already installed.

```bash
kubectl apply -f \
  https://github.com/kubernetes-sigs/gateway-api/releases/latest/download/standard-install.yaml
```

### 13.3 Register Kong as a Third-Party Gateway in WSO2 Admin Portal

**Purpose:** Declare Kong Gateway as an external (third-party) gateway inside WSO2 API Manager so that the control plane is aware of it for API discovery, visibility, and federated governance. Without this registration, WSO2 has no knowledge of the Kong runtime even if the gateway and its agent are running in the cluster.

1. Open the WSO2 Admin Portal and sign in.
2. Navigate to **Gateways**.
3. Click **Add Third-Party Gateway**.
4. Fill in the gateway details:
   - **Name:** `Kong`
   - **Display Name:** `Kong`
   - **Host:** `kong.wso2.com`
   - **HTTPS Port:** `8443`
   - **HTTP Port:** `8080`
   - **Type / Mode:** Third-Party (external gateway)
   - **Visibility:** `PUBLIC`
5. Save the configuration.

This registration does *not* deploy anything on the Kong side; it only creates the control-plane record that later allows the Kong Agent to associate discovered APIs with this gateway environment.

### 13.4 Add the Kong Certificate in the WSO2 Key Manager

**Purpose:** Configure the certificate used by the Kong gateway integration in WSO2 API Manager. This certificate allows the WSO2 control plane and the external Kong gateway configuration to trust the relevant public certificate material used for secure communication and token-related validation.

1. Open the WSO2 Admin Portal and sign in.
2. Navigate to **Key Managers**.
3. Open the key manager configuration associated with the Kong gateway integration.
4. Scroll to the **Certificates** section.
5. Upload the certificate.
6. Save the key manager configuration.

**Verification:** Reopen the key manager configuration and confirm that the certificate is still present under the **Certificates** section with **PEM** selected.

> **Note:** Only the public certificate must be added here. Private keys must never be pasted into the WSO2 Admin Portal or stored in plain text.

### 13.5 Install the Kong Operator

**Purpose:** Deploy the Kong Gateway Operator which manages the lifecycle of Kong data-plane instances based on Kubernetes Gateway API resources.

```bash
helm repo add kong https://charts.konghq.com
helm repo update

helm upgrade --install kong-operator kong/kong-operator \
  -n kong-system \
  --create-namespace \
  --set image.tag=2.2 \
  --set global.webhooks.options.certManager.enabled=true
```

**Verification:**

```bash
kubectl get pods -n kong-system
```

### 13.6 Deploy Kong Gateway Resources

**Purpose:** Create the `GatewayConfiguration`, `GatewayClass`, and `Gateway` resources that instruct the Kong Operator to provision a Kong data-plane instance.

```bash
kubectl apply -f gateways/kong/manifests/gateway-configuration.yaml
kubectl apply -f gateways/kong/manifests/gateway-class.yaml
kubectl apply -f gateways/kong/manifests/gateway.yaml
```

**Verification:**

```bash
kubectl get gatewayclass
kubectl get gateway   -n kong
kubectl get dataplane -n kong
kubectl get pods      -n kong
kubectl get svc       -n kong
```

### 13.7 Install the Kong Agent

**Purpose:** Deploy the common agent for Kong that enables API discovery and synchronisation between the Kong data plane and WSO2 API Manager.

```bash
helm install kong-agent common-agent/helm \
  -n kong \
  -f agents/kong-agent-values.yaml
```

**Verification:**

```bash
kubectl get pods -n kong
```

### 13.8 Deploy an Echo API on Kong

**Purpose:** Deploy a test `HTTPRoute` on Kong to verify end-to-end request routing through the Kong data plane.

```bash
kubectl apply -f gateways/kong/echo-api.yaml
```

Apply the CORS plugin if required:

```bash
kubectl apply -f gateways/kong/plugins/echo-cors.yaml
```

**Verification:** It should be discovered and found in the WSO2 publisher.

```bash
curl -k -X GET 'https://kong.wso2.com:8443/echo/' -H 'accept: */*'
```

---

## 14. Data Plane — Envoy Gateway

### 14.1 Create the Envoy Namespace

```bash
kubectl create namespace envoy
```

### 14.2 Deploy the Envoy Data-Plane Gateway

**Purpose:** Create a dedicated `Gateway` resource in the `envoy` namespace. The Envoy Gateway controller (installed earlier) provisions an Envoy Proxy instance for this gateway.

```bash
kubectl apply -f gateways/envoy/gateway.yaml
```

### 14.3 Register Envoy Gateway as a Third-Party Gateway in WSO2 Admin Portal

**Purpose:** Declare Envoy Gateway as an external (third-party) gateway inside WSO2 API Manager so that the control plane can discover, track, and govern APIs running on the Envoy Gateway data plane.

1. Open the WSO2 Admin Portal and sign in.
2. Navigate to **Gateways** (or **Gateway Environments**).
3. Click **Add Gateway Environment** / **Add Third-Party Gateway**.
4. Fill in the gateway details:
   - **Name:** `EG`
   - **Display Name:** `EG`
   - **Type / Environment:** `Envoy`
   - **Host:** `envoy.wso2.com`
   - **HTTPS Port:** `9443`
   - **HTTP Port:** `9080`
   - **Visibility:** `PUBLIC`
5. Save the configuration.

This step establishes the administrative record in the control plane required for the Envoy Agent to pair discovered APIs with the corresponding runtime environment.

### 14.4 Install the Envoy Agent

**Purpose:** Deploy the common agent for Envoy that enables API discovery and synchronisation between the Envoy data plane and WSO2 API Manager.

```bash
helm install envoy-agent ./common-agent/helm \
  -n envoy \
  -f agents/envoy-agent-values.yaml
```

Copy the agent root certificate to the `cert-manager` namespace so that the control plane trusts the agent:

```bash
kubectl get secret envoy-agent-root-certificate -n envoy -o yaml \
  | sed 's/namespace: envoy/namespace: cert-manager/' \
  | kubectl apply -f -
```

**Verification:**

```bash
kubectl get pods -n envoy
```

### 14.5 Deploy an Echo Backend and HTTPRoute

**Purpose:** Deploy a test backend service and its corresponding `HTTPRoute` to verify traffic routing through the Envoy data-plane gateway.

```bash
kubectl apply -f gateways/envoy/echo.yaml
kubectl apply -f gateways/envoy/httproute.yaml
```

Apply the CORS policy:

```bash
kubectl apply -f gateways/envoy/policies/cors-policy.yaml
```

**Verification:** It should be discovered and found in the WSO2 publisher.

```bash
curl -k -X GET 'https://envoy.wso2.com:9443/echo/' -H 'accept: */*'
```

---

## 15. Data Plane — MuleSoft Omni/Flex Gateway

**Purpose:** Deploy MuleSoft Flex Gateway as an additional data-plane gateway to demonstrate the extensibility of the hybrid architecture. Flex Gateway is a lightweight, high-performance API gateway from the MuleSoft Anypoint Platform that can run in connected mode under the control of Anypoint API Manager.

### 15.1 Register Flex Gateway with Anypoint Platform

**Purpose:** Pull the Flex Gateway Docker image and register a new gateway instance with the MuleSoft Anypoint Platform. The registration creates a `registration.yaml` file that authenticates the gateway with Anypoint API Manager in connected mode.

```bash
docker pull mulesoft/flex-gateway

docker run --entrypoint flexctl -u $UID \
  -v "$(pwd)":/registration mulesoft/flex-gateway \
  registration create \
    --organization=d80b9047-caae-4b17-8ffb-532bdfdabe82 \
    --token=1cdae92b-0950-4c3c-a8aa-b47dc9dfefc0 \
    --output-directory=/registration \
    --connected=true \
    flex
```

This produces a `registration.yaml` file in the current directory that is consumed by the Helm chart in the next step.

### 15.2 Install Flex Gateway via Helm

**Purpose:** Deploy Flex Gateway inside the Kubernetes cluster in *connected* mode, meaning it continuously synchronises its configuration with Anypoint API Manager.

```bash
helm repo add flex-gateway \
  https://flex-packages.anypoint.mulesoft.com/helm
helm repo update

helm -n gateway upgrade -i --create-namespace ingress \
  flex-gateway/flex-gateway \
  --set-file registration.content=registration.yaml \
  --set gateway.mode=connected
```

After initial deployment, subsequent upgrades use the project-local configuration files:

```bash
helm -n gateway upgrade ingress flex-gateway/flex-gateway \
  --set-file registration.content=gateways/flex/registration.yaml \
  -f gateways/flex/values-flex.yaml
```

**Verification:**

```bash
kubectl get pods -n gateway
```

### 15.3 Deploy the Test Backend

**Purpose:** Deploy the echo backend service that Flex Gateway will route traffic to.

```bash
kubectl apply -f gateways/flex/test-backend.yaml
```

### 15.4 Create and Deploy the API in Anypoint API Manager

**Purpose:** Publish the Echo API from Anypoint API Manager so that Flex Gateway (running in connected mode) receives the configuration and starts routing traffic to the Kubernetes echo backend. In connected mode, the gateway does not hold static API definitions locally; it pulls them from Anypoint Platform.

1. Open Anypoint Platform and sign in: https://anypoint.mulesoft.com
2. Navigate to **API Manager**.
3. Click **Add API** → **Add new API**.
4. Select **Flex Gateway** as the runtime.
5. Choose the registered gateway instance named `flex` (created during the `flexctl registration create` step).
6. Configure the API:
   - **API name:** `Echo API`
   - **Asset type:** REST API (or HTTP API)
   - **Version:** `1.0.0`
   - **Implementation URI** (upstream): the cluster-internal echo backend, e.g. `http://echo-backend.gateway.svc.cluster.local`
   - **Base path:** `/` (or `/echo` if a context path is required)
7. Save and deploy. Anypoint API Manager pushes the API configuration to Flex Gateway over the connected-mode control channel.
8. Optionally attach runtime policies from API Manager (rate limiting, CORS, JWT validation, client ID enforcement) according to the evaluation scenario.

**Verification:** Confirm that the API instance is in the **Active** / **Running** state in API Manager, then invoke the gateway:

```bash
curl -v http://flex-gateway.com:8280/
```

A successful response from the echo backend confirms that Anypoint API Manager → Flex Gateway → backend routing is operational.

### 15.5 WSO2 API Manager Integration — Research & Future Work

**Purpose:** Explore the feasibility of integrating MuleSoft Flex Gateway into the WSO2-centric control plane so that APIs governed by WSO2 API Manager can be deployed and enforced on Flex Gateway at runtime.

In this model, a **custom Anypoint Gateway Agent** — implemented as a Maven-based Java project — would act as a bridge between WSO2 API Manager and the Anypoint Platform. The agent would:

- Listen for API deployment events from WSO2 API Manager.
- Translate WSO2 API definitions into Anypoint-compatible API specifications.
- Push those specifications to Anypoint API Manager via its REST APIs.
- Anypoint API Manager would then propagate the configuration to Flex Gateway in connected mode.

**Current status:** This integration was **researched and designed** during the internship but was **not implemented**.

> **Key takeaway:** WSO2 API Manager's extensible architecture *does* allow this kind of third-party gateway integration through custom agents and connectors. The fact that a MuleSoft Flex Gateway can theoretically be managed from the same WSO2 control plane that already governs Kong and Envoy gateways opens significant horizons for truly **omni-gateway federated governance** — a single pane of glass managing APIs across any gateway technology, regardless of vendor.

---

## 16. Observability — Moesif Integration

**Purpose:** Integrate Moesif API analytics into all three data-plane gateways to obtain a unified, cross-gateway view of API traffic, latency, error rates, and consumer behaviour. Moesif is a SaaS API observability platform that ingests structured API-call events and exposes them through real-time dashboards and time-series analytics.

### 16.1 Moesif Account Setup

1. Go to https://www.moesif.com and sign up for a new account (or log in to an existing one).
2. Follow the onboarding wizard to create a workspace.
3. Navigate to **API Keys** and copy the **Moesif Application ID** (hereafter referred to as `moesifKey`). This key authenticates all event ingestion requests.

### 16.2 WSO2 Universal Gateway

#### 16.2.1 How It Works

The integration between WSO2 API Manager and Moesif is *native*. WSO2 uses the dedicated Moesif analytics reporter (`org.wso2.am.analytics.publisher.reporter.moesif`) to send API events directly to the Moesif ingestion API.

The pipeline is straightforward:

1. **Request enters the gateway.** The Universal Gateway processes the request normally through Synapse, including authentication, throttling, routing, and backend processing.
2. **Analytics events are generated.** WSO2 handlers capture request, response, and fault information such as API name, response code, latency, and correlation ID.
3. **Events are published asynchronously.** Events are placed in an in-memory queue and processed by a separate worker, keeping analytics outside the request/response critical path.
4. **Moesif reporter sends the events.** The reporter batches and sends events directly to Moesif's ingestion API over HTTPS using the configured `moesifKey`.
5. **Moesif processes the data.** The events are indexed and used to populate metrics such as requests, latency, error rate, and unique consumers.

#### 16.2.2 Configuration

Edit the `deployment.toml` file of the WSO2 API Manager control plane (mounted via the Helm values or baked into the custom Docker image) and add the following block:

```toml
[apim.analytics]
enable = true
type = "moesif"
moesifKey = "<YOUR_MOESIF_APPLICATION_ID>"
send_headers = true
```

- **`enable = true`** — Activates the analytics pipeline.
- **`type = "moesif"`** — Selects the Moesif reporter instead of the default ELK/OpenSearch reporter.
- **`moesifKey`** — The Application ID copied from the Moesif dashboard.
- **`send_headers = true`** — Includes HTTP request and response headers in the events sent to Moesif, enabling header-level filtering and debugging in the dashboard.

After updating the configuration, restart the control plane so the new analytics pipeline takes effect. The Universal Gateway will begin streaming events to Moesif on the next API call.

### 16.3 Kong Gateway

#### 16.3.1 How It Works

Kong integrates with Moesif through the official [Moesif plugin](https://developer.konghq.com/plugins/moesif/). Unlike the WSO2 native reporter, the Kong approach requires building a custom gateway image that bundles the plugin, because the Kong Operator deploys data-plane pods from a specified image.

The plugin intercepts every request/response cycle at the Kong proxy layer, captures the same metadata (status code, latency, headers, body size, consumer identity), and asynchronously batches events to the Moesif ingestion API — conceptually identical to the WSO2 pipeline but implemented as a Lua plugin inside the Kong/OpenResty runtime.

#### 16.3.2 Build and Push the Custom Kong Image

**Purpose:** Create a Kong Gateway Docker image that includes the Moesif plugin so the Kong Operator can deploy data-plane instances with analytics enabled out of the box.

```bash
docker build -t mayess03/kong-gateway-moesif:3.9 .
docker push mayess03/kong-gateway-moesif:3.9
```

**Verification:** The push output confirms the image digest (`sha256:f7565532...`).

#### 16.3.3 Update the Gateway Configuration

**Purpose:** Point the Kong `GatewayConfiguration` to the custom image so that newly provisioned data-plane pods include the Moesif plugin.

```bash
kubectl apply -f gateways/kong/manifests/gateway-configuration.yaml
```

#### 16.3.4 Deploy the Moesif Plugin

**Purpose:** Apply the Moesif plugin resource to the Kong data plane. The plugin configuration contains the `moesifKey` and any filtering rules (e.g. which routes or services to monitor).

```bash
kubectl apply -f gateways/kong/plugins/moesif-plugin.yaml
```

**Verification:**

```bash
kubectl get kongplugin -n kong
curl -k -X GET 'https://kong.wso2.com:8443/echo/' -H 'accept: */*'
```

Within a few seconds the API call should appear in the Moesif dashboard.

### 16.4 MuleSoft Flex Gateway

#### 16.4.1 How It Works

Flex Gateway does not have a native Moesif plugin. Instead, the integration uses **OpenTelemetry** as an intermediary tracing layer:

1. Flex Gateway is configured to export OpenTelemetry traces and metrics for every API call it processes.
2. An OpenTelemetry Collector (deployed as a sidecar or standalone service) receives these traces.
3. The collector forwards the trace data to the Moesif ingestion API using Moesif's OpenTelemetry-compatible endpoint.
4. Moesif maps the OpenTelemetry spans to its standard API-call model, populating the same dashboard fields (latency, status code, consumer identity) as the other two gateways.

This approach keeps the Flex Gateway configuration vendor-neutral while still feeding into the unified Moesif dashboard.

#### 16.4.2 Deploy the OpenTelemetry Tracing Configuration

**Purpose:** Apply the OpenTelemetry tracing manifest that configures Flex Gateway to export spans and routes them to Moesif.

```bash
kubectl apply -f gateways/flex/moesif-tracing.yaml
```

**Verification:**

```bash
kubectl get pods -n gateway
curl -v http://flex-gateway.com:8280/
```

### 16.5 Unified Cross-Gateway Dashboard

**Purpose:** Compare API traffic across all three gateways from a single Moesif workspace. Because every gateway sends events to the same Moesif application (using the same `moesifKey`), Moesif automatically aggregates the data.

The initial dashboard includes:

- **Total Requests** — broken down by gateway source (Universal, Kong, Flex) using metadata tags or the `X-Gateway` header injected by each gateway.
- **Average Latency** — per-gateway time-series comparing end-to-end response times.

Additional metrics such as *Error Rate*, *Unique Consumers*, *Throttled Requests*, and *Backend Latency vs. Proxy Latency* can be added incrementally. The dashboard is designed to be extensible: any new gateway integrated into the platform simply needs to send events to the same Moesif application to appear alongside the existing three.

> **Key takeaway:** By routing analytics from three heterogeneous gateways into a single observability platform, the architecture achieves *federated observability* that mirrors its federated governance model. Operators can compare gateway performance, detect anomalies, and make data-driven decisions about which gateway technology best suits each API workload — all from one pane of glass.

---

## 17. Observability — On-Premise Analytics (ELK Stack)

**Purpose:** As an alternative to the SaaS-based Moesif integration, WSO2 API Manager also ships a native **On-Premise Analytics** solution. Instead of streaming events to an external SaaS endpoint, the Universal Gateway and control plane write analytics events to a local log file, which is then shipped, indexed, and visualised by a self-hosted ELK stack. This keeps all API traffic data inside the cluster, which is a common requirement for Open Banking-style deployments with data-residency constraints.

The ELK-based On-Premise Analytics deployment architecture has four main components:

- **Filebeat** — tails the analytics log file on the gateway pod and forwards new lines to Logstash.
- **Logstash** — parses and transforms the raw log lines into structured events and indexes them into Elasticsearch.
- **Elasticsearch** — stores and indexes the structured API analytics events.
- **Kibana** — provides the dashboards and visualisations on top of the Elasticsearch indices.

This section covers the steps required to configure WSO2 API Manager to publish analytics to a log file, and to deploy an ELK cluster inside the platform's Kubernetes cluster to consume it. **This integration is currently configured for the WSO2 Universal Gateway only.**

### 17.1 Configure API Manager for ELK Analytics

#### 17.1.1 Enable the ELK Analytics Reporter

**Purpose:** Switch the `apim.analytics` reporter type from `moesif` to `elk` so that API Manager writes analytics events to a local log file instead of streaming them to an external SaaS endpoint.

Edit the `deployment.toml` file under `wso2am-4.x.x/repository/conf` (mounted via the Helm values or baked into the custom Docker image used for the control plane) and add the following block:

```toml
[apim.analytics]
enable = true
type = "elk"
```

**Applied configuration:** For this deployment, the following block was added to the `deployment.toml` of the control plane (`cp`):

```toml
[apim.analytics]
enable = true
type = "elk"
```

#### 17.1.2 Enable Metrics Logging

**Purpose:** Configure a dedicated log appender and logger so that the ELK analytics reporter writes its events to a rolling log file (`apim_metrics.log`) that Filebeat can later tail.

> **Note:** From WSO2 API-M 4.3.0 onwards, these configurations are added by default. They are listed here for completeness and in case they need to be re-applied to a custom image.

Open the `wso2am-4.x.x/repository/conf` directory and edit the `log4j2.properties` file as follows.

Add `APIM_METRICS_APPENDER` to the appenders list:

```properties
appenders = APIM_METRICS_APPENDER, .... (list of other available appenders)
```

Add the following appender configuration:

```properties
appender.APIM_METRICS_APPENDER.type = RollingFile
appender.APIM_METRICS_APPENDER.name = APIM_METRICS_APPENDER
appender.APIM_METRICS_APPENDER.fileName = ${sys:carbon.home}/repository/logs/apim_metrics.log
appender.APIM_METRICS_APPENDER.filePattern = ${sys:carbon.home}/repository/logs/apim_metrics-%d{MM-dd-yyyy}-%i.log
appender.APIM_METRICS_APPENDER.layout.type = PatternLayout
appender.APIM_METRICS_APPENDER.layout.pattern = %d{HH:mm:ss,SSS} [%X{ip}-%X{host}] [%t] %5p %c{1} %m%n
appender.APIM_METRICS_APPENDER.policies.type = Policies
appender.APIM_METRICS_APPENDER.policies.time.type = TimeBasedTriggeringPolicy
appender.APIM_METRICS_APPENDER.policies.time.interval = 1
appender.APIM_METRICS_APPENDER.policies.time.modulate = true
appender.APIM_METRICS_APPENDER.policies.size.type = SizeBasedTriggeringPolicy
appender.APIM_METRICS_APPENDER.policies.size.size=1000MB
appender.APIM_METRICS_APPENDER.strategy.type = DefaultRolloverStrategy
appender.APIM_METRICS_APPENDER.strategy.max = 10
```

Add a `reporter` logger to the loggers list:

```properties
loggers = reporter, ...(list of other available loggers)
```

Add the following logger configuration:

```properties
logger.reporter.name = org.wso2.am.analytics.publisher.reporter.elk
logger.reporter.level = INFO
logger.reporter.additivity = false
logger.reporter.appenderRef.APIM_METRICS_APPENDER.ref = APIM_METRICS_APPENDER
```

**Applied configuration:** For this deployment, the corresponding appender and logger blocks above were added directly to the `log4j2.properties` file used by the Universal Gateway image.

After updating both files, restart (or redeploy) the affected pods so the new logging pipeline takes effect.

#### 17.1.3 Verify Log Generation

**Purpose:** Confirm that the Universal Gateway is writing analytics events to `apim_metrics.log` before wiring up the ELK stack.

```bash
kubectl exec -n apim-cp apim-gw-wso2am-universal-gw-deployment-6d9bffcc66-cnm7f -- \
  cat /home/wso2carbon/wso2am-universal-gw-4.7.0/repository/logs/apim_metrics.log
```

**Verification:** The command should print one structured log line per API invocation handled by the Universal Gateway since the pod started.

### 17.2 Deploy the ELK Stack

**Purpose:** Provision Elasticsearch, Logstash, Filebeat, and Kibana inside a dedicated `elk` namespace so that the Universal Gateway's analytics log file can be ingested, indexed, and visualised entirely on-premise.

#### 17.2.1 Create the ELK Namespace

```bash
kubectl create namespace elk
```

#### 17.2.2 Deploy Elasticsearch

**Purpose:** Deploy the Elasticsearch instance that stores and indexes the structured API analytics events. A `PersistentVolumeClaim` is attached so indexed data survives pod restarts.

```bash
kubectl apply -f elk/elasticsearch.yaml
kubectl apply -f elk/elasticsearch-pvc.yaml
```

**Verification:**

```bash
kubectl get pods -n elk -w
curl http://localhost:9200
```

#### 17.2.3 Deploy Logstash

**Purpose:** Deploy Logstash together with its pipeline configuration. Logstash receives the raw log lines forwarded by Filebeat, parses them into structured fields, and indexes the resulting events into Elasticsearch.

```bash
kubectl apply -f elk/logstash-config.yaml
kubectl apply -f elk/logstash.yaml
```

#### 17.2.4 Deploy Filebeat

**Purpose:** Deploy Filebeat, which tails the `apim_metrics.log` file on the Universal Gateway pod(s) and ships new log lines to Logstash for parsing.

```bash
kubectl apply -f elk/filebeat.yaml
```

#### 17.2.5 Deploy Kibana

**Purpose:** Deploy Kibana, which provides the dashboards and visualisations on top of the Elasticsearch indices populated by Logstash.

```bash
kubectl apply -f elk/kibana.yaml
```

**Verification:** Confirm that all ELK components are running before moving on:

```bash
kubectl get pods -n elk
```

### 17.3 Access Kibana and Import Dashboards

**Purpose:** Reach the Kibana UI from outside the cluster and load the saved dashboard objects used to visualise Universal Gateway analytics.

Port-forward the Kibana service:

```bash
kubectl port-forward -n elk svc/kibana 5601:5601
```

Then open Kibana in a browser:

```
http://localhost:5601
```

From the Kibana UI, the previously exported saved objects file (`old_export.ndjson`) was imported to restore the existing index patterns, visualisations, and dashboards.

> **Note:** If the Kibana pod is restarted or rescheduled, its pod name changes; port-forward directly to the pod instead of the service when the service selector has not yet picked up the new pod, e.g.:
>
> ```bash
> kubectl port-forward -n elk pod/kibana-86f7fc85f5-wgl96 5601:5601
> ```

> **Key takeaway:** The ELK-based On-Premise Analytics path gives the platform a fully self-hosted alternative to the Moesif SaaS integration, keeping API traffic data inside the cluster end to end — from the Universal Gateway's `apim_metrics.log` file, through Filebeat and Logstash, into Elasticsearch, and finally visualised in Kibana. This integration currently covers the **WSO2 Universal Gateway only**; extending it to Kong, Envoy, and Flex Gateway is left for future work, following the same log-shipping pattern once each gateway's access logs are routed to Filebeat.