# Hybrid Federated API Gateway Platform

WSO2 API Manager Control Plane federating Kong Gateway, Envoy Gateway, and
WSO2's own Universal Gateway as independent data planes on Kubernetes (k3d).

See **ARCHITECTURE.md** for the full design reference: repo layout,
namespace design, port/hostname ownership, secrets handling, chart
versions, and lessons learned. Read it before making any structural change.

## Quick Start

```powershell
# 1. Create the cluster (ports pre-mapped for CP, Kong, Envoy — see PORTS.md)
k3d cluster create --config shared/k3d-cluster-config.yaml

# 2. Add hosts file entries — see PORTS.md for the full list
#    (edit C:\Windows\System32\drivers\etc\hosts as Administrator)

# 3. Install Gateway API CRDs
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/latest/download/standard-install.yaml

# 4. Install cert-manager, then issue the shared wildcard cert (shared/certs/)

# 5. Copy every *.example.yaml to its real (git-ignored) counterpart and
#    fill in actual secrets before installing anything:
#    control-plane/values.example.yaml       -> control-plane/values.yaml
#    gateways/*/install-values.example.yaml  -> gateways/*/install-values.yaml
#    agents/*-agent-values.example.yaml      -> agents/*-agent-values.yaml

# 6. Follow install order in ARCHITECTURE.md section 8:
#    CP -> Universal Gateway -> Kong (+ test route +  agent) -> Envoy (+ test route + agent)
```

## Status

Phase: **0 — repo & cluster scaffolding**

- [ ] Cluster created
- [ ] CRDs + cert-manager installed
- [ ] Shared wildcard cert issued
- [ ] Control Plane installed & verified standalone
- [ ] Universal Gateway installed & verified
- [ ] Kong installed, test route verified, agent installed & discovery verified
- [ ] Envoy installed, test route verified, agent installed & discovery verified
- [ ] Phase 4 — CP/GW distribution
