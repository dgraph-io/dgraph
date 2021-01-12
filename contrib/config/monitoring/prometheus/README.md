## Prometheus Metrics

[Prometheus](https://prometheus.io/) platform for gathering metrics and triggering alerts.  This can be used to monitor Dgraph deployed on the Kubernetes platform.

You can install [Prometheus](https://prometheus.io/) using either of these options:

* Kubernetes manifests (this directory)
  * Instructions: [Deploy: Monitoring in Kubernetes](https://dgraph.io/docs/deploy/#monitoring-in-kubernetes)
* Helm Chart Values - This will install [Prometheus](https://prometheus.io/), [AlertManager](https://prometheus.io/docs/alerting/latest/alertmanager/), and [Grafana](https://grafana.com/).
  * Instructions: [README.md](chart-values/README.md)

## Kubernetes Manifests Details

These manifests require the [prometheus-operator](https://coreos.com/blog/the-prometheus-operator.html) to be installed before using these (see [instructions](https://dgraph.io/docs/deploy/#monitoring-in-kubernetes)).

This will contain the following files:

* `prometheus.yaml` - Prometheus service and Dgraph service monitors to keep the configuration synchronized Dgraph configuration changes.  The service monitor use service discovery, such as Kubernetes labels and namespaces, to discover Dgraph.  Should you have multiple Dgraph installations installed, such as a dev-test and production, you can tailor these narrow the scope of which Dgraph version you would want to track.
* `alertmanager-config.yaml` - This is a secret you can create when installing `alertmanager.yaml`.  Here you can specify where to direct alerts, such as Slack or PagerDuty.
* `alertmanager.yaml` - AlertManager service to trigger alerts if metrics fall over a threshold specified in alert rules.
* `alert-rules.yaml` - These are rules that can trigger alerts. Adjust these as they make sense for your Dgraph deployment.
