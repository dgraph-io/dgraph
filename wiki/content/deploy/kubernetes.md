+++
date = "2017-03-20T22:25:17+11:00"
title = "Using Kubernetes"
weight = 6
[menu.main]
    parent = "deploy"
+++

The following section covers running Dgraph with Kubernetes. We have tested Dgraph with Kubernetes versions 1.14 to 1.16 on [GKE](https://cloud.google.com/kubernetes-engine) and versions 1.14 to 1.17 on [EKS](https://aws.amazon.com/eks/).

{{% notice "note" %}}These instructions are for running Dgraph alpha service without TLS configuration.
Instructions for running Dgraph alpha service with TLS refer [TLS instructions]({{< relref "deploy/tls-configuration.md" >}}).{{% /notice %}}

* Install [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) which is used to deploy
  and manage applications on kubernetes.
* Get the Kubernetes cluster up and running on a cloud provider of your choice.
  * For Amazon [EKS](https://aws.amazon.com/eks/), you can use [eksctl](https://eksctl.io/) to quickly provision a new cluster. If you are new to this, Amazon has an article [Getting started with eksctl](https://docs.aws.amazon.com/eks/latest/userguide/getting-started-eksctl.html).
  * For Google Cloud [GKE](https://cloud.google.com/kubernetes-engine), you can use [Google Cloud SDK](https://cloud.google.com/sdk/install) and the `gcloud container clusters create` command to quickly provision a new cluster.

Verify that you have your cluster up and running using `kubectl get nodes`. If you used `eksctl` or `gcloud container clusters create` with the default options, you should have 2-3 worker nodes ready.

On Amazon [EKS](https://aws.amazon.com/eks/), you would see something like this:

```sh
➜  kubernetes git:(master) ✗ kubectl get nodes
NAME                                          STATUS   ROLES    AGE   VERSION
<aws-ip-hostname>.<region>.compute.internal   Ready    <none>   1m   v1.15.11-eks-af3caf
<aws-ip-hostname>.<region>.compute.internal   Ready    <none>   1m   v1.15.11-eks-af3caf
```

On Google Cloud [GKE](https://cloud.google.com/kubernetes-engine), you would see something like this:

```sh
➜  kubernetes git:(master) ✗ kubectl get nodes
NAME                                       STATUS   ROLES    AGE   VERSION
gke-<cluster-name>-default-pool-<gce-id>   Ready    <none>   41s   v1.14.10-gke.36
gke-<cluster-name>-default-pool-<gce-id>   Ready    <none>   40s   v1.14.10-gke.36
gke-<cluster-name>-default-pool-<gce-id>   Ready    <none>   41s   v1.14.10-gke.36
```

## Single Server

Once your Kubernetes cluster is up, you can use [dgraph-single.yaml](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/kubernetes/dgraph-single/dgraph-single.yaml) to start a Zero, Alpha, and Ratel UI services.

### Deploy Single Server

From your machine, run the following command to start a StatefulSet that creates a single Pod with Zero, Alpha, and Ratel UI running in it.

```sh
kubectl create --filename https://raw.githubusercontent.com/dgraph-io/dgraph/master/contrib/config/kubernetes/dgraph-single/dgraph-single.yaml
```

Output:
```
service/dgraph-public created
statefulset.apps/dgraph created
```

### Verify Single Server

Confirm that the pod was created successfully.

```sh
kubectl get pods
```

Output:
```
NAME       READY     STATUS    RESTARTS   AGE
dgraph-0   3/3       Running   0          1m
```

{{% notice "tip" %}}
You can check the logs for the containers in the pod using
`kubectl logs --follow dgraph-0 <container_name>`. For example, try
`kubectl logs --follow dgraph-0 alpha` for server logs.
{{% /notice %}}

### Test Single Server Setup

Port forward from your local machine to the pod

```sh
kubectl port-forward pod/dgraph-0 8080:8080
kubectl port-forward pod/dgraph-0 8000:8000
```

Go to `http://localhost:8000` and verify Dgraph is working as expected.

### Remove Single Server Resources

Delete all the resources

```sh
kubectl delete --filename https://raw.githubusercontent.com/dgraph-io/dgraph/master/contrib/config/kubernetes/dgraph-single/dgraph-single.yaml
kubectl delete persistentvolumeclaims --selector app=dgraph
```

## HA Cluster Setup Using Kubernetes

This setup allows you to run 3 Dgraph Alphas and 3 Dgraph Zeros. We start Zero with `--replicas
3` flag, so all data would be replicated on 3 Alphas and form 1 alpha group.

{{% notice "note" %}} Ideally you should have at least three worker nodes as part of your Kubernetes
cluster so that each Dgraph Alpha runs on a separate worker node.{{% /notice %}}

### Validate Kubernetes Cluster for HA

Check the nodes that are part of the Kubernetes cluster.

```sh
kubectl get nodes
```

Output for Amazon [EKS](https://aws.amazon.com/eks/):

```sh
NAME                                          STATUS   ROLES    AGE   VERSION
<aws-ip-hostname>.<region>.compute.internal   Ready    <none>   1m   v1.15.11-eks-af3caf
<aws-ip-hostname>.<region>.compute.internal   Ready    <none>   1m   v1.15.11-eks-af3caf
<aws-ip-hostname>.<region>.compute.internal   Ready    <none>   1m   v1.15.11-eks-af3caf
```

Output for Google Cloud [GKE](https://cloud.google.com/kubernetes-engine)

```sh
NAME                                       STATUS   ROLES    AGE   VERSION
gke-<cluster-name>-default-pool-<gce-id>   Ready    <none>   41s   v1.14.10-gke.36
gke-<cluster-name>-default-pool-<gce-id>   Ready    <none>   40s   v1.14.10-gke.36
gke-<cluster-name>-default-pool-<gce-id>   Ready    <none>   41s   v1.14.10-gke.36
```

Once your Kubernetes cluster is up, you can use [dgraph-ha.yaml](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/kubernetes/dgraph-ha/dgraph-ha.yaml) to start the cluster.

#### Deploy Dgraph HA Cluster

From your machine, run the following command to start the cluster.

```sh
kubectl create --filename https://raw.githubusercontent.com/dgraph-io/dgraph/master/contrib/config/kubernetes/dgraph-ha/dgraph-ha.yaml
```

Output:
```sh
service/dgraph-zero-public created
service/dgraph-alpha-public created
service/dgraph-ratel-public created
service/dgraph-zero created
service/dgraph-alpha created
statefulset.apps/dgraph-zero created
statefulset.apps/dgraph-alpha created
deployment.apps/dgraph-ratel created
```

### Verify Dgraph HA Cluster

Confirm that the pods were created successfully.

It may take a few minutes for the pods to come up.

```sh
kubectl get pods
```

Output:
```sh
NAME                  READY   STATUS    RESTARTS   AGE
dgraph-alpha-0        1/1     Running   0          6m24s
dgraph-alpha-1        1/1     Running   0          5m42s
dgraph-alpha-2        1/1     Running   0          5m2s
dgraph-ratel-<pod-id> 1/1     Running   0          6m23s
dgraph-zero-0         1/1     Running   0          6m24s
dgraph-zero-1         1/1     Running   0          5m41s
dgraph-zero-2         1/1     Running   0          5m6s
```

{{% notice "tip" %}}You can check the logs for the containers in the pod using `kubectl logs --follow dgraph-alpha-0` and `kubectl logs --follow dgraph-zero-0`.{{% /notice %}}


### Test Dgraph HA Cluster Setup
Port forward from your local machine to the pod

```sh
kubectl port-forward service/dgraph-alpha-public 8080:8080
kubectl port-forward service/dgraph-ratel-public 8000:8000
```

Go to `http://localhost:8000` and verify Dgraph is working as expected.

{{% notice "note" %}} You can also access the service on its External IP address.{{% /notice %}}

### Delete Dgraph HA Cluster Resources

Delete all the resources

```sh
kubectl delete --filename https://raw.githubusercontent.com/dgraph-io/dgraph/master/contrib/config/kubernetes/dgraph-ha/dgraph-ha.yaml
kubectl delete persistentvolumeclaims --selector app=dgraph-zero
kubectl delete persistentvolumeclaims --selector app=dgraph-alpha
```

## Using Helm Chart

Once your Kubernetes cluster is up, you can make use of the Helm chart present
[in our official helm repository here](https://github.com/dgraph-io/charts/) to bring
up a Dgraph cluster.

{{% notice "note" %}}The instructions below are for Helm versions >= 3.x.{{% /notice %}}

### Installing the Chart

To add the Dgraph helm repository:

```sh
helm repo add dgraph https://charts.dgraph.io
```

To install the chart with the release name `my-release`:

```sh
helm install my-release dgraph/dgraph
```

The above command will install a recent version of the dgraph docker image. You can set the version an explicit version, such as:

```sh
helm install my-release dgraph/dgraph --set image.tag="v1.2.6"
```

{{% notice "warning" %}}When configuring dgraph image tag, be careful not to use `latest` or `master` in a production environment. These tags may have the dgraph version change, causing a mixed version dgraph cluster that can lead to an outage and potential data loss.{{% /notice %}}

#### Dgraph Configuration Files

You can supply a dgraph config files (see [Config]({{< relref "deploy/config.md" >}})) for alpha and zero with Helm chart configuration values:

```yaml
# my-config-values.yaml
alpha:
  configFile:
    config.yaml: |
      alsologtostderr: true
      badger:
        compression_level: 3
        tables: mmap
        vlog: mmap
      postings: /dgraph/data/p
      wal: /dgraph/data/w
zero:
  configFile:
    config.yaml: |
      alsologtostderr: true
      wal: /dgraph/data/zw
```

And then install with alpha and zero configuration using this:

```sh
helm install my-release dgraph/dgraph --values my-config-values.yaml
```

#### Exposing Alpha and Ratel Services

By default zero and alpha services are exposed only within the kubernetes cluster as
kubernetes service type `ClusterIP`.

In order to expose the alpha service and ratel service publicly you can use kubernetes service type `LoadBalancer` or an Ingress resource.

##### LoadBalancer (Public Internet)

To use an external load balancer, set the service type to `LoadBalancer`. 

{{% notice "note" %}}For security purposes we recommend limiting access to any public endpoints, such as using a white list.{{% /notice %}}

You can expose alpha service to the Internet as follows:

```sh
helm install my-release dgraph/dgraph --set alpha.service.type="LoadBalancer"
```

Similarly, you can expose alpha and ratel service to the Internet as follows:

```sh
helm install my-release dgraph/dgraph --set alpha.service.type="LoadBalancer" --set ratel.service.type="LoadBalancer"
```

##### LoadBalancer (Private Internal Network)

An external load balancer can be configured to face internally to a private subnet rather the public Internet.  This way it can be accessed securely by clients on the same network, through a VPN, or from a jump server. In Kubernetes, this is often configured through service annotations by the provider.  Here's a small list of annotations from cloud providers:

|Provider    | Documentation Reference   | Annotation |
|------------|---------------------------|------------|
|AWS         |[Amazon EKS: Load Balancing](https://docs.aws.amazon.com/eks/latest/userguide/load-balancing.html)|`service.beta.kubernetes.io/aws-load-balancer-internal: "true"`|
|Azure       |[AKS: Internal Load Balancer](https://docs.microsoft.com/azure/aks/internal-lb)|`service.beta.kubernetes.io/azure-load-balancer-internal: "true"`|
|Google Cloud|[GKE: Internal Load Balancing](https://cloud.google.com/kubernetes-engine/docs/how-to/internal-load-balancing)|`cloud.google.com/load-balancer-type: "Internal"`|


As an example, using Amazon [EKS](https://aws.amazon.com/eks/) as the provider, you could create a Helm chart configuration values like this below: 

```yaml
# my-config-values.yaml
alpha:
  service:
    type: LoadBalancer
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-internal: "true"
ratel:
  service:
    type: LoadBalancer
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-internal: "true"
```

And then expose alpha and ratel services privately:

```sh
helm install my-release dgraph/dgraph --values my-config-values.yaml
```

##### Ingress Resource

You can expose alpha and ratel using an [ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/) resource that can route traffic to service resources.  Before using this option you may need to install an [ingress controller](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/) first, as is the case with [AKS](https://docs.microsoft.com/azure/aks/) and [EKS](https://aws.amazon.com/eks/), while in the case of [GKE](https://cloud.google.com/kubernetes-engine), this comes bundled with a default ingress controller.  When routing traffic based on the `hostname`, you may want to integrate an addon like [ExternalDNS](https://github.com/kubernetes-sigs/external-dns) so that DNS records can be registered automatically when deploying dgraph.

As an examples, you can configure a single ingress resource that uses [ingress-nginx](https://github.com/kubernetes/ingress-nginx) for alpha and ratel services, by creating Helm chart configuration values like this below:

```yaml
# my-config-values.yaml
global:
  ingress:
    enabled: false
    annotations:
      kubernetes.io/ingress.class: nginx
    ratel_hostname: "ratel.<my-domain-name>"
    alpha_hostname: "alpha.<my-domain-name>"
```

And then expose alpha and ratel services through an ingress:

```sh
helm install my-release dgraph/dgraph --values my-config-values.yaml
```

Afterward you can run `kubectl get ingress` to see the status and access these through their hostname, such as `http://alpha.<my-domain-name>` and `http://ratel.<my-domain-name>`


{{% notice "tip" %}}Ingress controllers will likely have an option to configure access for private internal networks.  Consult documentation from the ingress controller provider for further information.{{% /notice %}}




### Upgrading the Chart

You can update your cluster configuration by updating the configuration of the
Helm chart. Dgraph is a stateful database that requires some attention on
upgrading the configuration carefully in order to update your cluster to your
desired configuration.

In general, you can use [`helm upgrade`][helm-upgrade] to update the
configuration values of the cluster. Depending on your change, you may need to
upgrade the configuration in multiple steps following the steps below.

[helm-upgrade]: https://helm.sh/docs/helm/helm_upgrade/

#### Upgrade to HA cluster setup

To upgrade to an [HA cluster setup]({{< relref "#ha-cluster-setup-using-kubernetes" >}}), ensure
that the shard replication setting is more than 1. When `zero.shardReplicaCount`
is not set to an HA configuration (3 or 5), follow the steps below:

1. Set the shard replica flag on the Zero node group. For example: `zero.shardReplicaCount=3`.
2. Next, run the Helm upgrade command to restart the Zero node group:
   ```sh
   helm upgrade my-release dgraph/dgraph [options]
   ```
3. Now set the Alpha replica count flag. For example: `alpha.replicaCount=3`.
4. Finally, run the Helm upgrade command again:
   ```sh
   helm upgrade my-release dgraph/dgraph [options]
   ```


### Deleting the Chart

Delete the Helm deployment as normal

```sh
helm delete my-release
```
Deletion of the StatefulSet doesn't cascade to deleting associated PVCs. To delete them:

```sh
kubectl delete pvc --selector release=my-release
```

### Configuration

The latest configuration settings can be found:

 * https://github.com/dgraph-io/charts/blob/master/charts/dgraph/README.md#configuration

## Monitoring in Kubernetes

Dgraph exposes prometheus metrics to monitor the state of various components involved in the cluster, this includes dgraph alpha and zero.

Below are instructions to setup Prometheus monitoring for your cluster.  This solution has the following parts:

* [prometheus-operator](https://coreos.com/blog/the-prometheus-operator.html) - a Kubernetes operator to install and configure Prometheus and Alert Manager.
* [Prometheus](https://prometheus.io/) - the service that will scrape Dgraph for metrics
* [AlertManager](https://prometheus.io/docs/alerting/latest/alertmanager/) - the service that will trigger alerts to a service (Slack, PagerDuty, etc) that you specify based on metrics exceeding threshold specified in Alert rules.
* [Grafana](https://grafana.com/) - optional visualization solution that will use Prometheus as a source to create dashboards.

### Installation through Manifests

Follow the below mentioned steps to setup prometheus monitoring for your cluster.

#### Install Prometheus operator

```sh
kubectl apply --filename https://raw.githubusercontent.com/coreos/prometheus-operator/release-0.34/bundle.yaml
```

Ensure that the instance of `prometheus-operator` has started before continuing.

```sh
$ kubectl get deployments prometheus-operator
NAME                  DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
prometheus-operator   1         1         1            1           3m
```

#### Install Prometheus

* Apply prometheus manifest present [here](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/monitoring/prometheus/prometheus.yaml).

```sh
$ kubectl apply --filename prometheus.yaml

serviceaccount/prometheus-dgraph-io created
clusterrole.rbac.authorization.k8s.io/prometheus-dgraph-io created
clusterrolebinding.rbac.authorization.k8s.io/prometheus-dgraph-io created
servicemonitor.monitoring.coreos.com/alpha.dgraph-io created
servicemonitor.monitoring.coreos.com/zero-dgraph-io created
prometheus.monitoring.coreos.com/dgraph-io created
```

To view prometheus UI locally run:

```sh
kubectl port-forward prometheus-dgraph-io-0 9090:9090
```

The UI is accessible at port 9090. Open http://localhost:9090 in your browser to play around.

#### Registering Alerts and Installing Alert Manager

To register alerts from dgraph cluster with your prometheus deployment follow the steps below:

* Create a kubernetes secret containing alertmanager configuration. Edit the configuration file present [here](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/monitoring/prometheus/alertmanager-config.yaml)
with the required reciever configuration including the slack webhook credential and create the secret.

You can find more information about alertmanager configuration [here](https://prometheus.io/docs/alerting/configuration/).

```sh
$ kubectl create secret generic alertmanager-alertmanager-dgraph-io \
    --from-file=alertmanager.yaml=alertmanager-config.yaml

$ kubectl get secrets
NAME                                            TYPE                 DATA   AGE
alertmanager-alertmanager-dgraph-io             Opaque               1      87m
```

* Apply the [alertmanager](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/monitoring/prometheus/alertmanager.yaml) along with [alert-rules](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/monitoring/prometheus/alert-rules.yaml) manifest
to use the default configured alert configuration. You can also add custom rules based on the metrics exposed by dgraph cluster similar to [alert-rules](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/monitoring/prometheus/alert-rules.yaml)
manifest.

```sh
$ kubectl apply --filename alertmanager.yaml
alertmanager.monitoring.coreos.com/alertmanager-dgraph-io created
service/alertmanager-dgraph-io created

$ kubectl apply --filename alert-rules.yaml
prometheusrule.monitoring.coreos.com/prometheus-rules-dgraph-io created
```

### Install Using Helm Chart

There are Helm chart values that will install Prometheus, Alert Manager, and Grafana.

You will first need to add the `prometheus-operator` Helm chart:

```bash
$ helm repo add stable https://kubernetes-charts.storage.googleapis.com
```

Afterward you will want to copy the Helm chart values present [here](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/monitoring/prometheus/chart-values/dgraph-prometheus-operator.yaml) and edit them as appropriate, such as adding endpoints, adding alert rules, adjusting alert manager configuration, adding Grafana dashboard, etc.

Once ready, install this with the following:

```bash
$ helm install my-prometheus-release \
  --values dgraph-prometheus-operator.yaml \
  --set grafana.adminPassword='<put-secret-password-here>' \
  stable/prometheus-operator
```

**NOTE**: For security best practices, we want to keep secrets, such as the Grafana password outside of general configuration, so that it is not accidently checked into anywhere.  You can supply it through the command line, or create a seperate `secrets.yaml` that is never checked into a code repository:

```yaml
grafana:
  adminPassword: <put-secret-password-here>
```

Then you can install this in a similar fashion:

```bash
$ helm install my-prometheus-release \
  --values dgraph-prometheus-operator.yaml \
  --values secrets.yaml \
  stable/prometheus-operator
```


### Adding Dgraph Kubernetes Grafana Dashboard

You can use the Grafana dashboard present [here](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/monitoring/grafana/dgraph-kubernetes-grafana-dashboard.json).  You can import this dashboard and select the Prometheus data source installed earlier.

This will visualize all Dgraph Alpha and Zero Kubernetes Pods, using the regex pattern `"/dgraph-.*-[0-9]*$/`.  This can be changed by through the dashboard configuration and selecting the variable Pod.  This might be desirable when you have had multiple releases, and only want to visualize the current release.  For example, if you installed a new release `my-release-3` with the [Dgraph helm chart](https://github.com/dgraph-io/charts/), you can change the regex pattern to `"/my-release-3.*dgraph-.*-[0-9]*$/"` for the Pod variable.

## Kubernetes Storage

The Kubernetes configurations in the previous sections were configured to run
Dgraph with any storage type (`storage-class: anything`). On the common cloud
environments like AWS, GCP, and Azure, the default storage type are slow disks
like hard disks or low IOPS SSDs. We highly recommend using faster disks for
ideal performance when running Dgraph.

### Local storage

The AWS storage-optimized i-class instances provide locally attached NVMe-based
SSD storage which provide consistent very high IOPS. The Dgraph team uses
i3.large instances on AWS to test Dgraph.

You can create a Kubernetes `StorageClass` object to provision a specific type
of storage volume which you can then attach to your Dgraph pods. You can set up
your cluster with local SSDs by using [Local Persistent
Volumes](https://kubernetes.io/blog/2018/04/13/local-persistent-volumes-beta/).
This Kubernetes feature is in beta at the time of this writing (Kubernetes
v1.13.1). You can first set up an EC2 instance with locally attached storage.
Once it is formatted and mounted properly, then you can create a StorageClass to
access it.:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: <your-local-storage-class-name>
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
```

Currently, Kubernetes does not allow automatic provisioning of local storage. So
a PersistentVolume with a specific mount path should be created:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: <your-local-pv-name>
spec:
  capacity:
    storage: 475Gi
  volumeMode: Filesystem
  accessModes:
  - ReadWriteOnce
  persistentVolumeReclaimPolicy: Delete
  storageClassName: <your-local-storage-class-name>
  local:
    path: /data
  nodeAffinity:
    required:
      nodeSelectorTerms:
      - matchExpressions:
        - key: kubernetes.io/hostname
          operator: In
          values:
          - <node-name>
```

Then, in the StatefulSet configuration you can claim this local storage in
.spec.volumeClaimTemplate:

```yaml
kind: StatefulSet
...
 volumeClaimTemplates:
  - metadata:
      name: datadir
    spec:
      accessModes:
      - ReadWriteOnce
      storageClassName: <your-local-storage-class-name>
      resources:
        requests:
          storage: 500Gi
```

You can repeat these steps for each instance that's configured with local
node storage.

### Non-local persistent disks

EBS volumes on AWS and PDs on GCP are persistent disks that can be configured
with Dgraph. The disk performance is much lower than locally attached storage
but can be sufficient for your workload such as testing environments.

When using EBS volumes on AWS, we recommend using Provisioned IOPS SSD EBS
volumes (the io1 disk type) which provide consistent IOPS. The available IOPS
for AWS EBS volumes is based on the total disk size. With Kubernetes, you can
request io1 disks to be provisioned with this config with 50 IOPS/GB using the
`iopsPerGB` parameter:

```
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: <your-storage-class-name>
provisioner: kubernetes.io/aws-ebs
parameters:
  type: io1
  iopsPerGB: "50"
  fsType: ext4
```

Example: Requesting a disk size of 250Gi with this storage class would provide
12.5K IOPS.

## Removing a Dgraph Pod

In the event that you need to completely remove a pod (e.g., its disk got
corrupted and data cannot be recovered), you can use the `/removeNode` API to
remove the node from the cluster. With a Kubernetes StatefulSet, you'll need to
remove the node in this order:

1. On the Zero leader, call `/removeNode` to remove the Dgraph instance from
   the cluster (see [More about Dgraph Zero]({{< relref
   "/deploy/dgraph-zero" >}})). The removed instance will immediately stop
   running. Any further attempts to join the cluster will fail for that instance
   since it has been removed.
2. Remove the PersistentVolumeClaim associated with the pod to delete its data.
   This prepares the pod to join with a clean state.
3. Restart the pod. This will create a new PersistentVolumeClaim to create new
   data directories.

When an Alpha pod restarts in a replicated cluster, it will join as a new member
of the cluster, be assigned a group and an unused index from Zero, and receive
the latest snapshot from the Alpha leader of the group.

When a Zero pod restarts, it must join the existing group with an unused index
ID. The index ID is set with the `--idx` flag. This may require the StatefulSet
configuration to be updated.

## Kubernetes and Bulk Loader

You may want to initialize a new cluster with an existing data set such as data
from the [Dgraph Bulk Loader]({{< relref "deploy/fast-data-loading.md#bulk-loader" >}}). You can use [Init
Containers](https://kubernetes.io/docs/concepts/workloads/pods/init-containers/)
to copy the data to the pod volume before the Alpha process runs.

See the `initContainers` configuration in
[dgraph-ha.yaml](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/kubernetes/dgraph-ha/dgraph-ha.yaml)
to learn more.
