# OVH Bare Metal Cloud Controller Manager

A minimal Kubernetes Cloud Controller Manager (CCM) for OVH dedicated (bare metal) servers.

## What It Does

Sets **ExternalIP**, **ProviderID**, and **topology labels** on bare metal Kubernetes nodes by querying the OVH API for dedicated server details.

This CCM is designed to run **alongside** the OpenStack CCM in mixed clusters (OVH Public Cloud VPS + OVH dedicated servers).

## How It Works

1. Bare metal nodes start with `--cloud-provider=external` and the `node.cloudprovider.kubernetes.io/uninitialized` taint
2. The CCM watches for nodes with the `node.ovh.com/service-name` annotation
3. It queries the OVH API (`GET /dedicated/server/{serviceName}`) for the server's public IP, region, and availability zone
4. It sets:
   - `ExternalIP` → from `server.ip`
   - `ProviderID` → `ovh-baremetal://{serviceName}`
   - `topology.kubernetes.io/zone` → from `server.availabilityZone`
   - `topology.kubernetes.io/region` → from `server.region` (uppercased)
5. It removes the `uninitialized` taint, making the node schedulable

## Node Configuration (Talos Linux)

```yaml
machine:
  nodeAnnotations:
    node.ovh.com/service-name: "nsXXXXXX.ip-X-X-X.eu"
  kubelet:
    extraArgs:
      cloud-provider: external
  nodeTaints:
    node.cloudprovider.kubernetes.io/uninitialized: "true:NoSchedule"
```

## Installation

### Helm (recommended)

```bash
# From OCI registry
helm install ovh-baremetal-ccm \
  oci://ghcr.io/lighthouse-engineering/charts/ovh-baremetal-ccm \
  --version 0.1.0 \
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
  --version 0.1.0 \
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
| Node identification | Nova API lookup by name | `node.ovh.com/service-name` annotation |
| Manages | VPS instances | Dedicated servers |
| ExternalIP source | OpenStack metadata | OVH dedicated server API |
| ProviderID format | `openstack:///{uuid}` | `ovh-baremetal://{serviceName}` |

Nodes without the `node.ovh.com/service-name` annotation are ignored by this CCM.

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
