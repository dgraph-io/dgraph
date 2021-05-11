# Binary Backups to Azure Blob

Binary backups can use Azure Blob Storage for object storage using [MinIO Azure Gateway](https://docs.min.io/docs/minio-gateway-for-azure.html).

## Provisioning Azure Blob

Some example scripts have been provided to illustrate how to create Azure Blob.

* [azure_cli](azure_cli/README.md) - shell scripts to provision Azure Blob
* [terraform](terraform/README.md) - terraform scripts to provision Azure Blob

## Setting up the Environment

### Prerequisites

You will need these tools:

* Docker Environment
  * [Docker](https://docs.docker.com/get-docker/) - container engine platform
  * [Docker Compose](https://docs.docker.com/compose/install/) - orchestrates running dokcer containers
* Kubernetes Environment
  * [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) - required for interacting with Kubenetes platform
  * [helm](https://helm.sh/docs/intro/install/) - deploys Kuberetes packages called helm charts
    * [helm-diff](https://github.com/databus23/helm-diff) [optional] - displays differences that will be applied to Kubernetes cluster
  * [helmfile](https://github.com/roboll/helmfile#installation) [optional] - orchestrates helm chart deployments

### Using Docker Compose

A `docker-compose.yml` configuration is provided that will run the [MinIO Azure Gateway](https://docs.min.io/docs/minio-gateway-for-azure.html) and Dgraph cluster.

#### Configuring Docker Compose

You will need to create a `minio.env` first:

```bash
MINIO_ACCESS_KEY=<azure-storage-account-name>
MINIO_SECRET_KEY=<azure-storage-account-key>
```

These values are used to both access the [MinIO Azure Gateway](https://docs.min.io/docs/minio-gateway-for-azure.html) using the same credentials used to access Azure Storage Account.  As a convenience, both example [Terraform](terraform/README.md) and [azure_cli](azure_cli/README.md) scripts will auto-generate the `minio.env`.

#### Using Docker Compose

```bash
## Run Minio Azure Gateway and Dgraph Cluster
docker-compose up --detach
```

#### Access Minio

* MinIO UI: http://localhost:9000

#### Clean Up Docker Environment

```bash
docker-compose stop
docker-compose rm
```

### Using Kubernetes with Helm Charts

For Kubernetes, you can deploy [MinIO Azure Gateway](https://docs.min.io/docs/minio-gateway-for-azure.html), Dgraph cluster, and a Kubernetes Cronjob that triggers backups using [helm](https://helm.sh/docs/intro/install/).

#### Configuring Secrets Values

These values are auto-generated if you used either [terraform](terraform/README.md) and [azure_cli](azure_cli/README.md) scripts.  If you already an existing Azure Blob you would like to use, you will need to create `charts/dgraph_secrets.yaml` and `charts/minio_secrets.yaml` files.

For the `charts/dgraph_secrets.yaml`, you would create a file like this:

```yaml
backups:
  keys:
    minio:
      access: <azure-storage-account-name>
      secret: <azure-storage-account-key>
```

For the `charts/minio_secrets.yaml`, you would create a file like this:

```yaml
accessKey: <azure-storage-account-name>
secretKey: <azure-storage-account-key>
```

#### Deploy Using Helmfile

If you have [helmfile](https://github.com/roboll/helmfile#installation) and [helm-diff](https://github.com/databus23/helm-diff) installed, you can deploy [MinIO Azure Gateway](https://docs.min.io/docs/minio-gateway-for-azure.html) and Dgraph cluster with the following:

```bash
export BACKUP_BUCKET_NAME=<name-of-bucket> # corresponds to Azure Container Name
helmfile apply
```
#### Deploy Using Helm

```bash
export BACKUP_BUCKET_NAME=<name-of-bucket> # corresponds to Azure Container Name
kubectl create namespace "minio"
helm repo add "minio" https://helm.min.io/
helm install "azuregw" \
  --namespace minio \
  --values ./charts/minio_config.yaml \
  --values ./charts/minio_secrets.yaml \
  minio/minio

helm repo add "dgraph" https://charts.dgraph.io
helm install "my-release" \
  --namespace default \
  --values ./charts/dgraph_config.yaml \
  --values ./charts/dgraph_secrets.yaml \
  --set backups.destination="minio://azuregw-minio.minio.svc:9000/${BACKUP_BUCKET_NAME}" \
  dgraph/dgraph
```

#### Access Resources

For MinIO UI, you can use this to access it at  http://localhost:9000:

```bash
export MINIO_POD_NAME=$(
 kubectl get pods \
  --namespace minio \
  --selector "release=azuregw" \
  --output jsonpath="{.items[0].metadata.name}"
)
kubectl --namespace minio port-forward $MINIO_POD_NAME 9000:9000
```

For Dgraph Alpha, you can use this to access it at http://localhost:8080:

```bash
export ALPHA_POD_NAME=$(
 kubectl get pods \
  --namespace default \
  --selector "statefulset.kubernetes.io/pod-name=my-release-dgraph-alpha-0,release=my-release" \
  --output jsonpath="{.items[0].metadata.name}"
)
kubectl --namespace default port-forward $ALPHA_POD_NAME 8080:8080
```

#### Cleanup Kubernetes Environment

If you are using helmfile, you can delete the resources with:

```bash
export BACKUP_BUCKET_NAME=<name-of-bucket> # corresponds ot Azure Container Name
helmfile delete
kubectl delete pvc --selector release=my-release # release dgraph name specified in charts/helmfile.yaml
```

If you are just helm, you can delete the resources with:

```bash
helm delete my-release --namespace default "my-release" # dgraph release name used earlier
kubectl delete pvc --selector release=my-release # dgraph release name used earlier
helm delete azuregw --namespace minio
```

## Triggering a Backup

This is run from the host with the alpha node accessible on localhost at port `8080`.  Can be done by running the docker-compose environment, or running `kubectl port-forward pod/dgraph-dgraph-alpha-0 8080:8080`.
In the docker-compose environment, the host for `MINIO_HOST` is `gateway`.  In the Kubernetes environment, using the scripts above, the `MINIO_HOST` is `azuregw-minio.minio.svc`.

### Using GraphQL

For versions of Dgraph that support GraphQL, you can use this:

```bash
ALPHA_HOST="localhost"  # hostname to connect to alpha1 container
MINIO_HOST="gateway"    # hostname from alpha1 container
BACKUP_BUCKET_NAME="<name-of-bucket>" # azure storage container name, e.g. dgraph-backups
BACKUP_PATH=minio://${MINIO_HOST}:9000/${BACKUP_BUCKET_NAME}?secure=false

GRAPHQL="{\"query\": \"mutation { backup(input: {destination: \\\"$BACKUP_PATH\\\" forceFull: true}) { response { message code } } }\"}"
HEADER="Content-Type: application/json"

curl --silent --header "$HEADER" --request POST $ALPHA_HOST:8080/admin --data "$GRAPHQL"
```

This should return a response in JSON that will look like this if successful:

```JSON
{
  "data": {
    "backup": {
      "response": {
        "message": "Backup completed.",
        "code": "Success"
      }
    }
  }
}
```

### Using REST API

For earlier Dgraph versions that support the REST admin port, you can do this:

```bash
ALPHA_HOST="localhost"  # hostname to connect to alpha1 container
MINIO_HOST="gateway"    # hostname from alpha1 container
BACKUP_BUCKET_NAME="<name-of-bucket>" # azure storage container name, e.g. dgraph-backups
BACKUP_PATH=minio://${MINIO_HOST}:9000/${BACKUP_BUCKET_NAME}?secure=false

curl --silent --request POST $ALPHA_HOST:8080/admin/backup?force_full=true --data "destination=$BACKUP_PATH"
```

This should return a response in JSON that will look like this if successful:

```JSON
{
  "code": "Success",
  "message": "Backup completed."
}
```
