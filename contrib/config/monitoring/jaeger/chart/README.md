# Jaeger Helm Chart

The Jaeger Helm Chart adds all components required to run Jaeger in Kubernetes for a production-like deployment.

## Tool Requirements

### Required

* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) - required to interact with kubernetes
* [helm](https://helm.sh/docs/intro/install/) - required to install jaeger-operator using helm chart

### Optional

These tools are optional if you would like to use a single command to install all the jaeger components and dgraph configured to use jaeger.

* [helmfile](https://github.com/roboll/helmfile#installation) (optional)
* [helm-diff](https://github.com/databus23/helm-diff) helm plugin: `helm plugin install https://github.com/databus23/helm-diff`

## Deploy

### Using Helmfile

```bash
helmfile apply
```

### Using Helm

```bash
kubectl create namespace observability

helm repo add jaegertracing https://jaegertracing.github.io/helm-charts
helm install "jaeger" \
  --namespace observability \
  --values ./jaeger_cassandra.yaml \
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

## Using Helm

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
