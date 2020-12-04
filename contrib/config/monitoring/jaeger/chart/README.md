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
export JAEGER_STORAGE_TYPE=cassandra
# ElasticSearch is the desired storage
export JAEGER_STORAGE_TYPE=elasticsearch
```

**IMPORTANT**: Change the `<secret_password>` to a strong password in the instructions below.

### Deploy Using Helmfile

```bash
JAEGER_STORAGE_PASSWORD="<secret_password>" helmfile apply
```

### Deploy Using Helm

```bash
kubectl create namespace observability

export JAEGER_STORAGE_TYPE=${JAEGER_STORAGE_TYPE:-'cassandra'}
helm repo add jaegertracing https://jaegertracing.github.io/helm-charts
helm install "jaeger" \
  --namespace observability \
  --values ./jaeger_${JAEGER_STORAGE_TYPE}.yaml \
  --set storage.${JAEGER_STORAGE_TYPE}.password="<secret_password>" \
  jaegertracing/jaeger

helm install "my-release" \
  --namespace default \
  --values ./dgraph_jaeger.yaml \
  dgraph/dgraph
```


## Cleanup

### Cleanup Using Helmfile

```bash
## Delete Jaeger, Storage (Cassandra or ElasticSearch), Dgraph
JAEGER_STORAGE_PASSWORD="<secret_password>" helmfile delete

## Remove Any Persistent Storage
kubectl delete pvc --namespace default --selector release="dgraph"
kubectl delete pvc --namespace observability --selector release="jaeger"

```

### Cleanup Using Helm

```bash
## Delete Jaeger, Storage (Cassandra or ElasticSearch), Dgraph
helm delete --namespace default "my-release"
helm delete --namespace observability "jaeger"

## Remove Any Persistent Storage
kubectl delete pvc --namespace default --selector release="my-release"
kubectl delete pvc --namespace observability --selector release="jaeger"
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
