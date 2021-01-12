# Helm Chart Values

You can install [Prometheus](https://prometheus.io/) and [Grafana](https://grafana.com/) using this helm chart and supplied helm chart values.

## Usage

### Tool Requirements

* [Kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) - Kubernetes client tool to interact with a Kubernetes cluster
* [Helm](https://helm.sh/) - package manager for Kubernetes
* [Helmfile](https://github.com/roboll/helmfile#installation) (optional) - declarative spec that allows you to compose several helm charts
  * [helm-diff](https://github.com/databus23/helm-diff) - helm plugin used by `helmfile` to show differences when applying helm files.

### Using Helm

You can use helm to install [kube-prometheus-stack](https://github.com/prometheus-operator/kube-prometheus) helm chart. This helm chart is a collection of Kubernetes manifests, [Grafana](http://grafana.com/) dashboards, , [Prometheus rules](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/) combined with scripts to provide monitoring with [Prometheus](https://prometheus.io/) using the [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator).  This helm chart will also install [Grafana](http://grafana.com/), [node_exporter](https://github.com/prometheus/node_exporter), [kube-state-metrics](https://github.com/kubernetes/kube-state-metrics).

To use this, run the following:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add stable https://charts.helm.sh/stable
helm repo update

## set grafanan secret admin password
GRAFANA_ADMIN_PASSWORD='<put-complete-password-here>'
## optionally set namespace (default=monitoring if not specified)
export NAMESPACE="monitoring"

helm install my-prometheus-release \
  --values ./dgraph-prometheus-operator.yaml \
  --set grafana.adminPassword=$GRAFANA_ADMIN_PASSWORD \
  --namespace $NAMESPACE \
  prometheus-community/kube-prometheus-stack
```

### Using Helmfile

You can use helmfile to manage multiple helm charts and corresponding helmcharts values from a single configuration file: `helmfile.yaml`.  The provided example `helmfile.yaml` will show how to use this to install the helm chart.  

To use this, run the following:

```bash
## set grafanan secret admin password
GRAFANA_ADMIN_PASSWORD='<put-complete-password-here>'
## optionally set namespace (default=monitoring if not specified)
export NAMESPACE="monitoring"

helmfile apply
```

## Grafana Dashboards

You can import [Grafana](https://grafana.com/) Dashboards from within the web consoles.  

There's an example dash board for some metrics that you can use to monitor Dgraph on Kubernetes:

* [dgraph-kubernetes-grafana-dashboard.json](../../grafana/dgraph-kubernetes-grafana-dashboard.json)

## Helm Chart Values

Here are some Helm chart values you may want to configure depending on your environment.

### General

* `grafana.service.type` - set to `LoadBalancer` if you would like to expose this port.
* `grafana.service.annotations` - add annotations to configure a `LoadBalancer` such as if it is internal or external facing, DNS name with external-dns, etc.
* `prometheus.service.type` - set to `LoadBalancer` if you would like to expose this port.
* `prometheus.service.annotations` - add annotations to configure a `LoadBalancer` such as if it is internal or external facing, DNS name with external-dns, etc.

### Dgraph Service Monitors

* `prometheus.additionalServiceMonitors.namespaceSelector.matchNames` - if you want to match a dgraph installed into a specific namespace.
* `prometheus.additionalServiceMonitors.selector.matchLabels` - if you want to match through a specific labels in your dgraph deployment.  Currently matches `monitor: zero.dgraph-io` and `monitor: alpha.dgraph-io`, which si the default for [Dgraph helm chart](https://github.com/dgraph-io/charts).

## Binary Backup Monitoring Support

If you enabled binary backups through Kubernetes CronJob exposed in Dgraph helm chart, you can use the examples here add monitoring of backup cron jobs.

With `helmfile`, you can deploy this using the following:

```bash
## set grafanan secret admin password
GRAFANA_ADMIN_PASSWORD='<put-complete-password-here>'
## optionally set namespace (default=monitoring if not specified)
export NAMESPACE="monitoring"
## enable backups
export DGRAPH_BACKUPS_ENABLED=1
## enable pagerduty and set integration key
export PAGERDUTY_INTEGRATION_KEY='<pagerduty-intregration-key-goes-here>'

helmfile apply
```

For PagerDuty integration, you will need to add a service with integration type of `Prometheus` and later copy the integration key that is created.

## Upgrading form previous versions

Previously, this chart was called `stable/prometheus-operator`, which has been deprecated and now called `prometheus-community/kube-prometheus-stack`.  You will have to do a migration from the old chart to the new chart.  The prometheus community has created a migration guide for this process:

* [Migrating from stable/prometheus-operator chart](https://github.com/prometheus-community/helm-charts/blob/main/charts/kube-prometheus-stack/README.md#migrating-from-stableprometheus-operator-chart)
