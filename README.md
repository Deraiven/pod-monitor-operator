# pod-monitor-operator

A Kubernetes Operator that monitors Pod container restarts and Linkerd certificate expiration, exposing metrics via Prometheus.

## Description

Pod Monitor Operator provides real-time monitoring capabilities for:
- Container restart events with detailed termination information (reason, exit code, timestamp)
- Linkerd identity certificate expiration monitoring
- Persistent event tracking that survives operator restarts

All metrics are exposed in Prometheus format for easy integration with monitoring stacks.

## Quick Start with Helm

```sh
# 1. Build and push image
make docker-build docker-push IMG=myregistry/pod-monitor:v1.0.0

# 2. Install using Helm
helm install pod-monitor ./pod-monitor \
  --set image.repository=myregistry/pod-monitor \
  --set image.tag=v1.0.0 \
  --namespace pod-monitor-system \
  --create-namespace

# 3. Check metrics
kubectl port-forward -n pod-monitor-system svc/pod-monitor 8080:8080
curl http://localhost:8080/metrics | grep pod_monitor
```

## Getting Started

### Prerequisites
- go version v1.23.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/pod-monitor-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/pod-monitor-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/pod-monitor-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/pod-monitor-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

Pod Monitor Operator includes a Helm chart for easy deployment. The chart is located in the `pod-monitor/` directory.

#### Prerequisites

1. **Build and push the Docker image first:**

```sh
# Build the image
make docker-build IMG=<your-registry>/pod-monitor-operator:tag

# Push to your registry
make docker-push IMG=<your-registry>/pod-monitor-operator:tag
```

2. **Ensure Helm 3 is installed:**

```sh
helm version
```

#### Installation using Helm

1. **Update the values.yaml with your image:**

```sh
# Edit pod-monitor/values.yaml
# Update the image repository and tag to match your pushed image
vi pod-monitor/values.yaml
```

Or override during installation:

```sh
# Install with custom image
helm install pod-monitor ./pod-monitor \
  --set image.repository=<your-registry>/pod-monitor-operator \
  --set image.tag=<your-tag> \
  --namespace pod-monitor-system \
  --create-namespace
```

2. **Install the chart:**

```sh
# Install from local directory
helm install pod-monitor ./pod-monitor \
  --namespace pod-monitor-system \
  --create-namespace

# Or with custom values file
helm install pod-monitor ./pod-monitor \
  -f my-values.yaml \
  --namespace pod-monitor-system \
  --create-namespace
```

3. **Verify the installation:**

```sh
# Check the deployment
kubectl get deployments -n pod-monitor-system

# Check the pods
kubectl get pods -n pod-monitor-system

# Check the service
kubectl get svc -n pod-monitor-system
```

#### Configuration Options

Key configuration options in `values.yaml`:

```yaml
# Image configuration
image:
  repository: <your-registry>/pod-monitor-operator
  tag: "latest"
  pullPolicy: IfNotPresent

# Resource limits
resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 10m
    memory: 64Mi

# Service configuration
service:
  type: ClusterIP
  port: 8080  # Metrics port

# Enable Linkerd monitoring
linkerdMonitoring:
  enabled: true
  namespace: linkerd
  secretName: linkerd-identity-issuer
```

#### Accessing Metrics

Once deployed, the metrics are available at:

```sh
# Port-forward to access metrics locally
kubectl port-forward -n pod-monitor-system svc/pod-monitor 8080:8080

# View metrics
curl http://localhost:8080/metrics | grep pod_monitor
```

#### Prometheus Configuration

Add the following scrape config to your Prometheus:

```yaml
scrape_configs:
  - job_name: 'pod-monitor'
    kubernetes_sd_configs:
      - role: service
        namespaces:
          names:
            - pod-monitor-system
    relabel_configs:
      - source_labels: [__meta_kubernetes_service_name]
        regex: pod-monitor
        action: keep
```

#### Upgrading

```sh
# Update values and upgrade
helm upgrade pod-monitor ./pod-monitor \
  --namespace pod-monitor-system \
  --set image.tag=<new-tag>
```

#### Uninstalling

```sh
# Remove the helm release
helm uninstall pod-monitor -n pod-monitor-system

# Delete the namespace (optional)
kubectl delete namespace pod-monitor-system
```

## Available Metrics

### Container Restart Metrics

- `pod_monitor_container_restart_total` - Total number of container restarts (Counter)
  - Labels: `namespace`, `pod`, `container`, `reason`

- `pod_monitor_container_restart_events` - Individual restart events with timestamps (Gauge)
  - Labels: `namespace`, `pod`, `container`, `reason`, `exit_code`, `restart_count`
  - Value: Unix timestamp of termination

- `pod_monitor_container_last_termination_info` - Last termination information (Gauge)
  - Labels: `namespace`, `pod`, `container`, `reason`, `exit_code`
  - Value: Unix timestamp of termination

### Certificate Expiration Metrics

- `pod_monitor_certificate_expiration_timestamp_seconds` - Certificate expiration timestamp (Gauge)
  - Labels: `namespace`, `secret_name`, `cert_type`
  - Value: Unix timestamp of expiration

- `pod_monitor_certificate_days_until_expiration` - Days until certificate expires (Gauge)
  - Labels: `namespace`, `secret_name`, `cert_type`
  - Value: Number of days remaining

## Example Prometheus Queries

```promql
# View all container restart events
pod_monitor_container_restart_events

# Container restart rate over last 5 minutes
rate(pod_monitor_container_restart_total[5m])

# Containers that restarted due to OOMKilled
pod_monitor_container_restart_events{reason="OOMKilled"}

# Certificates expiring in less than 30 days
pod_monitor_certificate_days_until_expiration < 30

# Total restarts by reason
sum by (reason) (pod_monitor_container_restart_total)
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

