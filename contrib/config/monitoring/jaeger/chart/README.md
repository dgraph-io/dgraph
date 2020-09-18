# Jaeger Helm Chart

The [Jaeger Helm Chart](https://github.com/jaegertracing/helm-charts/tree/master/charts/jaeger) adds all components required to run Jaeger in Kubernetes for a production-like deployment.

## Tool Requirements

### Required

* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) - required to interact with kubernetes
* [helm](https://helm.sh/docs/intro/install/) - required to install jaeger, cassandra, and elasticsearch using helm chart

### Optional

These tools are optional if you would like to use a single command to install all the jaeger components and dgraph configured to use jaeger.

* [helmfile](https://github.com/roboll/helmfile#installation) (optional)
* [helm-diff](https://github.com/databus23/helm-diff) helm plugin: `helm plugin install https://github.com/databus23/helm-diff`

## Deploy

First choose the desired storage of Cassandra or ElasticSearch:

```bash
# Cassandra is desired storage
export JAEGER_STORAGE=cassandra
# ElasticSearch is the desired storage
export JAEGER_STORAGE=elasticsearch
```

**IMPORTANT**: In either `jaeger_cassandra.yaml` or `jaeger_elasticsearch.yaml`, change the password helm config values to a strong password.

### Using Helmfile

```bash
helmfile apply
```

### Using Helm

```bash
kubectl create namespace observability

export JAEGER_STORAGE=${JAEGER_STORAGE:-'cassandra'}
helm repo add jaegertracing https://jaegertracing.github.io/helm-charts
helm install "jaeger" \
  --namespace observability \
  --values ./jaeger_${JAEGER_STORAGE}.yaml \
  jaegertracing/jaeger

helm install "my-release" \
  --namespace default \
  --values ./dgraph_jaeger.yaml \
  dgraph/dgraph
```

## Cleanup

### Using Helmfile

```bash
helmfile delete
```

### Using Helm

```bash
helm delete --namespace default "my-release"
kubectl delete pvc --namespace default --selector release="my-release"
helm delete --namespace observability "jaeger"
```

## Jaeger Query UI

```bash
export POD_NAME=$(kubectl get pods \
  --namespace observability \
  --selector "app.kubernetes.io/instance=jaeger,app.kubernetes.io/component=query" \
  --output jsonpath="{.items[0].metadata.name}"
)
kubectl port-forward --namespace observability $POD_NAME 16686:16686
```

Afterward, you can visit:

* http://localhost:16686
