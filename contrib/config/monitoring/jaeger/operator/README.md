# Jaeger Operator

The [Jaeger operator](https://github.com/jaegertracing/jaeger-operator) is an implementation of a [Kubernetes operator](https://coreos.com/operators/) that aims to ease the operational complexity of deploying and managing Jaeger.

## Tool Requirements

### Required

* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) - required to interact with kubernetes
* [helm](https://helm.sh/docs/intro/install/) - required to install jaeger-operator using helm chart

### Optional

These tools are optional if you would like to use a single command to install all the jaeger components and dgraph configured to use jaeger.

* [helmfile](https://github.com/roboll/helmfile#installation) 
* [helm-diff](https://github.com/databus23/helm-diff) helm plugin: `helm plugin install https://github.com/databus23/helm-diff`

## Deploy

### Deploy Using Helmfile

```bash
helmfile apply
```

### Deploy Helm and Kubectl

If you do not have `helmfile` available you can do these steps:

```bash
kubectl create namespace observability

## Install Jaeger Operator
helm repo add jaegertracing https://jaegertracing.github.io/helm-charts
helm install "jaeger-operator" \
 --namespace observability \
 --set serviceAccount.name=jaeger-operator \
 --set rbac.clusterRole=true \
 jaegertracing/jaeger-operator

## Install Jaeger using Jaeger Operator CRD
kubectl apply \
  --namespace observability \
  --kustomize ./jaeger-kustomize/overlays/badger

## Install Dgraph configured to use Jaeger
helm repo add dgraph https://charts.dgraph.io
helm install "my-release" \
  --namespace default \
  --values ./dgraph_jaeger.yaml \
  dgraph/dgraph
```

## Cleanup

### Cleanup Using Helmfile

```bash
helmfile delete
kubectl delete pvc --namespace default --selector release="dgraph"
```

### Cleanup Using Helm and Kubectl

```bash
## Delete Dgraph and Dgraph Persistence
helm delete --namespace default "my-release"
kubectl delete pvc --namespace default --selector release="my-release"

## Delete Jaeger
kubectl delete \
  --namespace observability \
  --kustomize jaeger-kustomize/overlays/badger/

## Delete Jaeger Operator
helm delete --namespace observability "jaeger-operator"
```

## Jaeger Query UI

You can use port-forward option to access the Jaeger Query UI from localhost with this:

```bash
export POD_NAME=$(kubectl get pods \
  --namespace observability \
  --selector "app.kubernetes.io/instance=jaeger,app.kubernetes.io/component=all-in-one" \
  --output jsonpath="{.items[0].metadata.name}"
)
kubectl port-forward --namespace observability $POD_NAME 16686:16686
```

Afterward, visit:

* http://localhost:16686
