# Networking / Port Ownership Reference

Keep this in sync with ARCHITECTURE.md section 3. This is the single source
of truth for hostnames, ports, and which component owns the shared k3d
LoadBalancer IP.

| Component | Hostname | HTTP Port | HTTPS Port | Owns shared LB IP? |
|---|---|---|---|---|
| WSO2 CP (mgmt/portal) | am.wso2.com | 80 | 443 | Yes |
| Universal Gateway | gw.wso2.com | 80 | 443 | Yes (shares CP's listener via SNI) |
| Kong | kong.wso2.com | 8080 | 8443 | No |
| Envoy | envoy.wso2.com | 9080 | 9443 | No |

## Local hosts file entries required

Add to `C:\Windows\System32\drivers\etc\hosts` (as Administrator):

```
127.0.0.1   am.wso2.com
127.0.0.1   gw.wso2.com
127.0.0.1   websocket.wso2.com
127.0.0.1   websub.wso2.com
127.0.0.1   km.wso2.com
127.0.0.1   kong.wso2.com
127.0.0.1   envoy.wso2.com
```
