# Hybrid Federated API Gateway Architecture — Reference

This document is the source of truth for the repo layout, namespace design,
networking, and operational decisions for this project. Update it whenever a
decision changes — it should always reflect what's actually deployed.

---

## 1. Repo Structure

```
repo/
├── control-plane/                  # WSO2 APIM Control Plane (wso2am-acp chart)
│   ├── values.example.yaml         # committed, placeholder secrets
│   └── values.yaml                 # git-ignored, real secrets
├── gateways/
│   ├── universal/                  # WSO2's native gateway (wso2am-gateway chart)
│   │   ├── install-values.example.yaml
│   │   ├── install-values.yaml     # git-ignored
│   │   └── manifests/              # APIs deployed directly via Publisher/APICTL
│   ├── kong/
│   │   ├── install-values.example.yaml
│   │   ├── install-values.yaml     # git-ignored
│   │   └── manifests/              # HTTPRoutes, backends, ReferenceGrants
│   └── envoy/
│       ├── install-values.example.yaml
│       ├── install-values.yaml     # git-ignored
│       └── manifests/
├── agents/
│   ├── kong-agent-values.example.yaml
│   ├── kong-agent-values.yaml      # git-ignored
│   ├── envoy-agent-values.example.yaml
│   ├── envoy-agent-values.yaml     # git-ignored
│   # note: no agent needed for universal — it's native, not federated
├── shared/
│   ├── certs/                      # cert-manager Issuer/Certificate manifests
│   ├── gateway-api-crds/
│   └── k3d-cluster-config.yaml     # cluster creation config, incl. port mappings
├── secrets/                        # git-ignored entirely; real values live here
├── PORTS.md                        # this doc's networking table, kept in sync
└── .gitignore                      # excludes all real values.yaml / secrets/
```

**Rule:** every Helm install is driven by a values file committed to the repo
(or its git-ignored counterpart) — never a bare `--set` typed live in a
terminal. If it's not in a file, it doesn't count as done.

---

## 2. Namespace Layout

| Namespace | Contains |
|---|---|
| `apim-cp` | WSO2 Control Plane (APIM core) + MySQL |
| `apim-gw-universal` | Universal Gateway data plane (native WSO2) |
| `apim-gw-kong` | Kong Gateway + its HTTPRoutes/backends |
| `apim-gw-envoy` | Envoy Gateway + its HTTPRoutes/backends |
| `apim-agent-kong` | Kong's federated discovery agent |
| `apim-agent-envoy` | Envoy's federated discovery agent |
| `envoy-gateway-system` | Envoy Gateway's own controller (chart-mandated, not renameable) |
| `cert-manager` | cert-manager, shared across everything |

### Why agents get dedicated namespaces (not colocated with anything else)

Two instances of the `apim-k8s-common-gw-helm` chart cannot share a namespace:
several resource names (ServiceAccount, cert-manager-issued Secret) are
hardcoded rather than templated per-release in the current (beta) chart.
Installing two releases in the same namespace causes Helm ownership
conflicts (`invalid ownership metadata`) regardless of using distinct
release names. Separate namespaces per agent sidesteps this entirely.

### `dataPlane.namespace` vs. agent pod namespace

These are independent settings and must not be confused:

- **Where the agent pod lives** = the namespace you `helm install ... -n <ns>` into
  (`apim-agent-kong` / `apim-agent-envoy`)
- **What the agent watches** = `dataPlane.namespace` in its values file
  (`apim-gw-kong` / `apim-gw-envoy` respectively)

### Gateway + HTTPRoute + backend colocation

**Decision:** Gateway object, its HTTPRoutes, and their backend Services all
live in the *same* namespace per gateway (e.g. everything Kong-related in
`apim-gw-kong`). This avoids needing `ReferenceGrant` objects for the common
case. Only reach for cross-namespace references + ReferenceGrants if there's
a specific, deliberate reason to split later — don't let it happen by accident.

---

## 3. Networking — Hostnames, Ports, and LB IP Ownership

| Component | Hostname | HTTP Port | HTTPS Port | Owns shared LB IP (172.18.0.x)? |
|---|---|---|---|---|
| WSO2 CP (mgmt/portal) | `am.wso2.com` | 80 | 443 | **Yes** |
| Universal Gateway | `gw.wso2.com` | 80 | 443 | **Yes** (shares the same Gateway/listener as CP via SNI — this already works natively; don't touch it) |
| Kong | `kong.wso2.com` | 8080 | 8443 | No — dedicated ports |
| Envoy | `envoy.wso2.com` | 9080 | 9443 | No — dedicated ports |

**Reasoning:** k3d's shared LoadBalancer IP can only bind one Gateway's Envoy
proxy per port. CP and Universal Gateway are what a person types into a
browser without thinking about ports, so they keep the shared IP on 80/443.
Kong and Envoy are federated data-planes serving their own APIs under
distinct hostnames, so they get explicit non-default ports — this removes
port contention as a *possible* failure mode entirely, rather than leaving it
to whichever Helm chart happens to install first.

This is encoded directly in cluster creation, not left to chance:

```yaml
# shared/k3d-cluster-config.yaml (excerpt)
ports:
  - port: 80:80
    nodeFilters: ["loadbalancer"]
  - port: 443:443
    nodeFilters: ["loadbalancer"]
  - port: 8080:8080
    nodeFilters: ["loadbalancer"]
  - port: 8443:8443
    nodeFilters: ["loadbalancer"]
  - port: 9080:9080
    nodeFilters: ["loadbalancer"]
  - port: 9443:9443
    nodeFilters: ["loadbalancer"]
```

Add corresponding entries to the local hosts file for every hostname above,
pointing at `127.0.0.1`.

---

## 4. Secrets Handling

Given this is a local dev/internship project (not a production/enterprise
deployment), the approach is deliberately simple rather than using
Vault/SOPS/sealed-secrets:

- Every values file containing real secrets (DB passwords, encryption key,
  admin credentials, TLS keys) is **git-ignored**.
- A matching `*.example.yaml` is committed alongside it, with placeholder
  values (e.g. `password: "CHANGE_ME"`) — this documents the shape of the
  config without leaking anything.
- Real files live in a single git-ignored `secrets/` folder or as the
  git-ignored `values.yaml` counterpart next to each `values.example.yaml`.
- Reference the real file explicitly at install time, e.g.
  `-f secrets/apim-cp-values.yaml`.
- `.gitignore` at repo root excludes: `**/values.yaml`,
  `**/install-values.yaml`, `**/*-agent-values.yaml`, `secrets/`.

---

## 5. Chart Version Pinning

Every chart version is pinned explicitly and recorded here. Upgrades are a
deliberate, tested decision — never an implicit `helm repo update` followed
by an unpinned install (this is exactly what silently corrupted an agent
config during initial setup).

| Chart | Repo | Version |
|---|---|---|
| WSO2 APIM Control Plane (`wso2am-acp`) | `wso2-cp` | 4.7.0-1 |
| WSO2 Universal Gateway (`wso2am-gateway`) | `wso2-gw` | 4.7.0-1 |
| Envoy Gateway (`gateway-helm`) | `oci://docker.io/envoyproxy/gateway-helm` | *(pin on install)* |
| Kong Ingress Controller | `kong` | *(pin on install)* |
| APIM Common Agent (`apim-k8s-common-gw-helm`) | `agent` | 1.0.0-beta |
| cert-manager | `agent` (or dedicated repo) | *(pin on install)* |

*(Fill in the two `(pin on install)` rows with the exact versions chosen
during Phase 1, and keep this table updated on every intentional upgrade.)*

---

## 6. TLS Certificate Strategy

**Decision:** one shared wildcard certificate (`*.wso2.com`) with a proper
SAN list covering every hostname in use, issued once via cert-manager, and
referenced by name from every gateway's HTTPS listener — rather than a
separate self-signed cert per gateway.

- SANs to include at minimum: `am.wso2.com`, `gw.wso2.com`,
  `websocket.wso2.com`, `websub.wso2.com`, `km.wso2.com`, `kong.wso2.com`,
  `envoy.wso2.com`
- Issued via a cert-manager `Certificate` + self-signed (or local CA)
  `Issuer`, defined in `shared/certs/`
- Stored as a single Kubernetes `Secret`, referenced identically from CP,
  Universal Gateway, Kong, and Envoy listener configs
- Avoids repeating the ad hoc self-signed-cert-per-gateway process; avoids
  CN-only certs with no SAN (modern clients ignore CN entirely and require
  SAN — a CN-only cert will fail TLS validation for every client except with
  `-k`/insecure flags)

---

## 7. Deploy-mode vs. Discovery-mode (per gateway) — OPEN DECISION

Still to be finalized before Phase 3 for Kong and Envoy:

- **Discovery-only**: HTTPRoutes/Services are hand-written directly in
  Kubernetes; the agent discovers them into WSO2, read-only.
- **Deploy-mode**: APIs are created in the WSO2 Publisher, which generates
  and pushes the HTTPRoute/config to the gateway directly.
- **Read-Write**: both directions active simultaneously.

This determines `agent.mode` (`CPtoDP` / other) in each agent's values file
and changes the day-to-day workflow for adding new APIs. Universal Gateway
is deploy-only by nature (native WSO2 target) and needs no such decision.

**Action:** decide per-gateway before writing `agents/kong-agent-values.yaml`
and `agents/envoy-agent-values.yaml`.

---

## 8. Install Order (Phase 1 → Phase 3 summary)

1. Create k3d cluster from `shared/k3d-cluster-config.yaml`
2. Install Gateway API CRDs
3. Install cert-manager
4. Issue the shared wildcard certificate (`shared/certs/`)
5. Install WSO2 Control Plane (`control-plane/`) — verify standalone, no
   gateway dependency yet
6. Install Universal Gateway (`gateways/universal/`) — verify `am.wso2.com` /
   `gw.wso2.com` work
7. Install Kong (`gateways/kong/install-values.yaml`) — deploy one trivial
   test backend + HTTPRoute in `apim-gw-kong` — verify raw traffic via curl
   on port 8080/8443 — **only then** install Kong's agent
   (`apim-agent-kong` namespace) — verify discovery in Publisher — commit as
   a known-good checkpoint
8. Repeat step 7 for Envoy on port 9080/9443, `apim-agent-envoy`
9. Move to Phase 4 (CP/GW distribution) only once both gateways are
   independently verified working end-to-end

---

## 9. Lessons Baked Into These Decisions

- Never reuse a Helm release name across different target configs — a
  same-name `helm upgrade` silently overwrote a working agent config with an
  unrelated one during initial setup.
- Two `apim-k8s-common-gw-helm` releases cannot share a namespace (hardcoded
  resource names) — always separate namespaces per agent.
- k3d's shared LoadBalancer IP binds one Gateway per port — decide port
  ownership explicitly at cluster-creation time.
- `ReferenceGrant` is only needed when Gateway/HTTPRoute/backend deliberately
  span namespaces — default to colocating them to avoid the complexity.
- CN-only TLS certs (no SAN) fail validation in virtually all modern
  clients — always issue certs with an explicit SAN list.
- Gateway API `parentRefs` without an explicit `namespace` field default to
  the *same namespace as the HTTPRoute* — easy to get wrong when copying
  examples from docs into a different namespace context.
