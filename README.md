# OVH Bare Metal Cloud Controller Manager

A minimal Kubernetes Cloud Controller Manager (CCM) for OVH dedicated (bare metal) servers.

## What It Does

Sets **ExternalIP**, **ProviderID**, and **topology labels** on bare metal Kubernetes nodes by querying the OVH API for dedicated server details.

This CCM is designed to run **alongside** the OpenStack CCM in mixed clusters (OVH Public Cloud VPS + OVH dedicated servers).

## How It Works

1. Bare metal nodes must have the **`node.ovh.com/service-name`** label set to their OVH dedicated server service name (e.g. `nsXXXXXX.ip-X-X-X.eu`). This label must be configured **before** the node joins the cluster — typically via the Talos machine config or equivalent provisioning tool. The CCM does not set this label; it only reads it.
2. Nodes must boot with `--cloud-provider=external` and the `node.cloudprovider.kubernetes.io/uninitialized` taint.
3. The CCM **only watches** nodes that carry the `node.ovh.com/service-name` label. Nodes without the label (e.g. VPS instances managed by the OpenStack CCM) are completely invisible to this controller — no API calls, no log entries, no errors.
4. For each labeled node, it queries the OVH API (`GET /dedicated/server/{serviceName}`) and sets:
   - `ExternalIP` → from `server.ip`
   - `ProviderID` → `ovh-baremetal://{serviceName}`
   - `topology.kubernetes.io/zone` → from `server.availabilityZone`
   - `topology.kubernetes.io/region` → from `server.region` (uppercased to match OpenStack CCM format)
5. It removes the `uninitialized` taint, making the node schedulable.

## Node Label

The `node.ovh.com/service-name` label is the **only** way the CCM identifies which nodes it should manage. You must set it yourself — the CCM never writes this label.

The label value must be the OVH dedicated server service name, which you can find in the OVH dashboard URL or via the OVH API (`GET /dedicated/server`). Examples: `nsXXXXXX.ip-X-X-X.eu`, `ns1234567.ip-192-168-1.eu`.

### Setting the label

**Talos Linux** (recommended — set at provisioning time):

```yaml
machine:
  nodeLabels:
    node.ovh.com/service-name: "nsXXXXXX.ip-X-X-X.eu"
  kubelet:
    extraArgs:
      cloud-provider: external
  nodeTaints:
    node.cloudprovider.kubernetes.io/uninitialized: "true:NoSchedule"
```

**kubectl** (for existing nodes — requires a kubelet restart with `--cloud-provider=external`):

```bash
kubectl label node <node-name> node.ovh.com/service-name=nsXXXXXX.ip-X-X-X.eu
```

## Installation

### Helm (recommended)

```bash
# From OCI registry
helm install ovh-baremetal-ccm \
  oci://ghcr.io/lighthouse-engineering/charts/ovh-baremetal-ccm \
  --version 0.3.0 \
  --namespace kube-system \
  --set ovh.applicationKey=YOUR_KEY \
  --set ovh.applicationSecret=YOUR_SECRET \
  --set ovh.consumerKey=YOUR_CONSUMER_KEY
```

### Helm with existing secret

```bash
# Create the secret first
kubectl create secret generic ovh-baremetal-ccm \
  --namespace kube-system \
  --from-literal=endpoint=ovh-eu \
  --from-literal=application_key=YOUR_KEY \
  --from-literal=application_secret=YOUR_SECRET \
  --from-literal=consumer_key=YOUR_CONSUMER_KEY

# Install with existingSecret
helm install ovh-baremetal-ccm \
  oci://ghcr.io/lighthouse-engineering/charts/ovh-baremetal-ccm \
  --version 0.3.0 \
  --namespace kube-system \
  --set existingSecret=ovh-baremetal-ccm
```

### Helm Values

| Key | Default | Description |
|---|---|---|
| `ovh.endpoint` | `ovh-eu` | OVH API endpoint |
| `ovh.applicationKey` | `""` | OVH API application key |
| `ovh.applicationSecret` | `""` | OVH API application secret |
| `ovh.consumerKey` | `""` | OVH API consumer key |
| `existingSecret` | `""` | Use an existing K8s Secret for OVH credentials |
| `image.repository` | `ghcr.io/lighthouse-engineering/ovh-baremetal-ccm` | CCM image |
| `image.tag` | Chart appVersion | Image tag |
| `replicaCount` | `1` | Number of replicas |
| `logVerbosity` | `2` | Log verbosity (0-10) |
| `nodeSelector` | `{node-role.kubernetes.io/control-plane: ""}` | Pod node selector |
| `resources.requests.cpu` | `50m` | CPU request |
| `resources.requests.memory` | `64Mi` | Memory request |
| `resources.limits.memory` | `128Mi` | Memory limit |

See [values.yaml](deploy/helm/ovh-baremetal-ccm/values.yaml) for all options.

## Coexistence with OpenStack CCM

This CCM runs alongside the OpenStack CCM without conflicts:

| Aspect | OpenStack CCM | Bare Metal CCM |
|---|---|---|
| Provider name | `openstack` | `ovh-baremetal` |
| Leader election lease | `cloud-controller-manager` | `ovh-baremetal-ccm` |
| Node identification | Nova API lookup by name | `node.ovh.com/service-name` label |
| Node filtering | Watches all nodes | Watches **only** labeled nodes |
| Manages | VPS instances | Dedicated servers |
| ExternalIP source | OpenStack metadata | OVH dedicated server API |
| ProviderID format | `openstack:///{uuid}` | `ovh-baremetal://{serviceName}` |

The informer-level label filter means the bare metal CCM never sees VPS nodes at all — no wasted API calls, no error logs, no interference.

## Building

```bash
# Build binary
go build -o ovh-baremetal-ccm ./cmd/ovh-baremetal-ccm/

# Build Docker image
docker build -t ghcr.io/lighthouse-engineering/ovh-baremetal-ccm:latest .

# Lint Helm chart
helm lint deploy/helm/ovh-baremetal-ccm/
```

## License

Apache License 2.0
