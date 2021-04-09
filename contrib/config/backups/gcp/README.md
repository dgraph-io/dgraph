# Binary Backups to Google Cloud Storage

Binary backups can use [Google Cloud Storage](https://cloud.google.com/storage) for object storage using [MinIO GCS Gateway](https://docs.min.io/docs/minio-gateway-for-gcs.html).

## Provisioning GCS

Some example scripts have been provided to illustrate how to create a bucket in GCS.

* [terraform](terraform/README.md) - terraform scripts to provision GCS bucket

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

A `docker-compose.yml` configuration is provided that will run the MinIO GCS gateway and Dgraph cluster.

#### Configuring Docker Compose

The Docker Compose configuration `docker-compose.yml` will require the following files:

 * `credentials.json` - credentials that grant access to the GCS bucket
 * `minio.env` - that holds `MINIO_ACCESS_KEY` and `MINIO_SECRET_KEY` values.
 * `env.sh` - tha stores `PROJECT_ID` and `BACKUP_BUCKET_NAME`.

For convenience, [terraform](terraform/README.md) scripts and generate a random password.

The `minio.env` will be used by both Dgraph alpha node(s) and the [MinIO GCS Gateway](https://docs.min.io/docs/minio-gateway-for-gcs.html) server. You will need to create a file like this:

```bash
# minio.env
MINIO_ACCESS_KEY=<my-access-key>
MINIO_SECRET_KEY=<my-secret-key>
```

The `env.sh` will be source before using Docker Compose or before triggering backups:

```bash
# env.sh
export PROJECT_ID=<project-id>
export BACKUP_BUCKET_NAME=<gcs-bucket-name>
```

#### Using Docker Compose

```bash
## source script for envvars: PROJECT_ID and BACKUP_BUCKET_NAME
. env.sh
## Run Minio GCS Gateway and Dgraph Cluster
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

For Kubernetes, you can deploy [MinIO GCS Gateway](https://docs.min.io/docs/minio-gateway-for-gcs.html), Dgraph cluster, and a Kubernetes Cronjob that triggers backups using [helm](https://helm.sh/docs/intro/install/).

#### Configuring Secrets Values

These values are generated if you used either [terraform](terraform/README.md) scripts.  If you already have an existing GCS bucket that you would like to use, you will need to create `charts/dgraph_secrets.yaml` and `charts/minio_secrets.yaml` files.

For the `charts/dgraph_secrets.yaml`, you would create a file like this:

```yaml
backups:
  keys:
    minio:
      access: <minio-access-key>
      secret: <minio-secret-key>
```

For the `charts/minio_secrets.yaml`, you would create a file like this:

```yaml
accessKey: <minio-access-key>
secretKey: <minio-secret-key>
gcsgateway:
  gcsKeyJson: |
    <contents-of-credentials-json>
```

#### Configuring Environments

Create an `env.sh` file to store `BACKUP_BUCKET_NAME` and `PROJECT_ID`.  If [terraform](terraform/README.md) scripts were used to create the GCS bucket, then these scripts will have already generated this file.

This is the same file used for the Docker Compose environment and will look like this:

```bash
# env.sh
export PROJECT_ID=<project-id>
export BACKUP_BUCKET_NAME=<gcs-bucket-name>
```

#### Deploy Using Helmfile

If you have [helmfile](https://github.com/roboll/helmfile#installation) and [helm-diff](https://github.com/databus23/helm-diff) installed, you can deploy [MinIO GCS Gateway](https://docs.min.io/docs/minio-gateway-for-gcs.html) and Dgraph cluster with the following:

```bash
## source script for envvars: PROJECT_ID and BACKUP_BUCKET_NAME
. env.sh
## deploy Dgraph cluster and MinIO GCS Gateway using helm charts
helmfile apply
```

#### Deploy Using Helm

```bash
## source script for envvars: PROJECT_ID and BACKUP_BUCKET_NAME
. env.sh
## deploy MinIO GCS Gateway in minio namespace
kubectl create namespace "minio"
helm repo add "minio" https://helm.min.io/
helm install "gcsgw" \
  --namespace minio \
  --values ./charts/minio_config.yaml \
  --values ./charts/minio_secrets.yaml \
  --set gcsgateway.projectId=${PROJECT_ID} \
  minio/minio

## deploy Dgraph in default namespace
helm repo add "dgraph" https://charts.dgraph.io
helm install "my-release" \
  --namespace "default" \
  --values ./charts/dgraph_config.yaml \
  --values ./charts/dgraph_secrets.yaml \
  --set backups.destination="minio://gcsgw-minio.minio.svc:9000/${BACKUP_BUCKET_NAME}" \
  dgraph/dgraph
```

#### Access Resources

For MinIO UI, you can use this to access it at  http://localhost:9000:

```bash
export MINIO_POD_NAME=$(
 kubectl get pods \
  --namespace minio \
  --selector "release=gcsgw" \
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
## source script for envvars: PROJECT_ID and BACKUP_BUCKET_NAME
. env.sh
## remove Dgraph cluster and MinIO GCS Gateway
helmfile delete
## remove storage used by Dgraph cluster
kubectl delete pvc --selector release=my-release # release dgraph name specified in charts/helmfile.yaml
```

If you are just helm, you can delete the resources with:

```bash
helm delete my-release --namespace default "my-release" # dgraph release name used earlier
kubectl delete pvc --selector release=my-release # dgraph release name used earlier
helm delete gcsgw --namespace minio
```

## Triggering a Backup

This is run from the host with the alpha node accessible on localhost at port `8080`.  Can be done by running the docker-compose environment, or running `kubectl port-forward pod/dgraph-dgraph-alpha-0 8080:8080`.
In the docker-compose environment, the host for `MINIO_HOST` is `gateway`.  In the Kubernetes environment, using the scripts above, the `MINIO_HOST` is `gcsgw-minio.minio.svc`.

### Using GraphQL

For versions of Dgraph that support GraphQL, you can use this:

```bash
## source script for envvars BACKUP_BUCKET_NAME
. env.sh
## variables based depending on docker or kubernetes env
ALPHA_HOST="localhost"  # hostname to connect to alpha1 container
MINIO_HOST="gateway"    # hostname from alpha1 container
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
## source script for envvars BACKUP_BUCKET_NAME
. env.sh
## variables based depending on docker or kubernetes env
ALPHA_HOST="localhost"  # hostname to connect to alpha1 container
MINIO_HOST="gateway"    # hostname from alpha1 container
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
