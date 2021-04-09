# Dgraph High Availability

`dgraph-ha.yaml` is an example manifest to deploy Dgraph cluster on Kubernetes:

* 3 zero nodes
* 3 alpha nodes

You can deploy the manifest with `kubectl`:

```bash
kubectl apply --filename dgraph-ha.yaml
```

## Accessing the Services

You can access services deploy from `dgraph-ha.yaml` running each of these commands in a separate terminal window or tab:

```bash
# port-forward to alpha
kubectl port-forward svc/dgraph-alpha-public 8080:8080
```

## Public Services

There are three services specified in the manifest that can be used to expose services outside the cluster.  Highly recommend that when doing this, they are only accessible on a private subnet, and not exposed to the public Internet.

* `dgraph-zero-public` - To load data using Live & Bulk Loaders
* `dgraph-alpha-public` - To connect clients and for HTTP APIs

For security best practices, these are set as `ClusterIP` service type, so they are only accessible from within the Kubernetes cluster.  If you need to expose these to outside of the cluster, there are a few options:

* Change the [service type](https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types) to `LoadBalancer` for the private intranet access.
* Install an [ingress controller](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/) that can provide access to the service from outside the Kubernetes cluster for private intranet access.

Configuring a service with `LoadBalancer` service type or an Ingress Controller to use a private subnet is very implementation specific. Here are some examples:

|Provider    | Documentation Reference   | Annotation |
|------------|---------------------------|------------|
|AWS         |[Amazon EKS: Load Balancing](https://docs.aws.amazon.com/eks/latest/userguide/load-balancing.html)|`service.beta.kubernetes.io/aws-load-balancer-internal: "true"`|
|Azure       |[AKS: Internal Load Balancer](https://docs.microsoft.com/en-us/azure/aks/internal-lb)|`service.beta.kubernetes.io/azure-load-balancer-internal: "true"`|
|Google Cloud|[GKE: Internal Load Balancing](https://cloud.google.com/kubernetes-engine/docs/how-to/internal-load-balancing)|`cloud.google.com/load-balancer-type: "Internal"`|




## Amazon EKS

### Create K8S using eksctl (optional)

An example cluster manifest for use with `eksctl` is provided quickly provision an Amazon EKS cluster.  

The `eksctl` tool can be installed following these instructions:

* https://docs.aws.amazon.com/eks/latest/userguide/getting-started-eksctl.html

Once installed, you can provision Amazon EKS with the following command (takes ~20 minutes):

```bash
eksctl create cluster --config-file cluster.yaml
```
