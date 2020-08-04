# Helm Chart Values

You can install [Prometheus](https://prometheus.io/) and [Grafana](https://grafana.com/) using this helm chart and supplied helm chart values. 

## Usage

```bash
helm repo add stable https://kubernetes-charts.storage.googleapis.com
helm install my-prometheus-release \
  --values dgraph-prometheus-operator.yaml \
  --set grafana.adminPassword='<put-complete-password-here>' \ 
  stable/prometheus-operator 
```

## Grafana Dashboards

You can import [Grafana](https://grafana.com/) Dashboards from within the web consoles.  

There's an example dash board for some metrics that you can use to monitor Dgraph on Kubernetes:

* [dgraph-kubernetes-grafana-dashboard.json](../../grafana/dgraph-kubernetes-grafana-dashboard.json)

## Helm Chart Values

Here are some Helm chart values you may want to configure depending on your environment.

## General

* `grafana.service.type` - set to `LoadBalancer` if you would like to expose this port.
* `grafana.service.annotations` - add annotations to configure a `LoadBalancer` such as if it is internal or external facing, DNS name with external-dns, etc.
* `prometheus.service.type` - set to `LoadBalancer` if you would like to expose this port.
* `prometheus.service.annotations` - add annotations to configure a `LoadBalancer` such as if it is internal or external facing, DNS name with external-dns, etc.

## Dgraph Service Monitors

* `prometheus.additionalServiceMonitors.namespaceSelector.matchNames` - if you want to match a dgraph installed into a specific namespace.
* `prometheus.additionalServiceMonitors.selector.matchLabels` - if you want to match through a specific labels in your dgraph deployment.  Currently matches `monitor: zero.dgraph-io` and `monitor: alpha.dgraph-io`, which si the default for [Dgraph helm chart](https://github.com/dgraph-io/charts).
