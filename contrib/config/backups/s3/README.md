# Binary Backups to S3

Binary backups can use AWS Simple Storage Service for object storage.

## Provisioning S3

Some example scripts have been provided to illustrate how to create S3.

* [terraform](terraform/README.md) - terraform scripts to provision S3 bucket and an IAM user with access to the bucket.

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

A `docker-compose.yml` configuration is provided that will run the Dgraph cluster.

#### Configuring Docker Compose

You will need to create a `s3.env` first the the example below. If you created this bucket with [terraform](terraform/README.md) scripts, this will be created automatically.

```bash
## s3.env
AWS_ACCESS_KEY_ID=<aws-access-key>
AWS_SECRET_ACCESS_KEY=<aws-secret-key>
```

#### Using Docker Compose

```bash
## Run a Dgraph Cluster
docker-compose up --detach
```

#### Access Ratel UI

* Ratel UI: http://localhost:8000

#### Clean Up Docker Environment

```bash
docker-compose stop
docker-compose rm
```

### Using Kubernetes with Helm Charts

For Kubernetes, you can deploy Dgraph cluster and a Kubernetes Cronjob that triggers backups using [helm](https://helm.sh/docs/intro/install/).

#### Configuring Secrets Values

These values are auto-generated if you used either [terraform](terraform/README.md).  If you already an existing S3 bucket you would like to use, you will need to create `charts/dgraph_secrets.yaml` files.

For the `charts/dgraph_secrets.yaml`, you would create a file like this:

```yaml
backups:
  keys:
    s3:
      access: <aws-access-key>
      secret: <aws-secret-key>
```

#### Configuring Environments

We need to define one environment variable `BACKUP_PATH`.  If [terraform](terraform/README.md) scripts were used to create the S3 bucket, we can source the `env.sh` or otherwise create it here:

```bash
## env.sh
export BACKUP_PATH=s3://s3.<region>.amazonaws.com/<s3-bucket-name>
```

#### Deploy Using Helmfile

If you have [helmfile](https://github.com/roboll/helmfile#installation) and [helm-diff](https://github.com/databus23/helm-diff) installed, you can deploy a Dgraph cluster with the following:

```bash
## source script for envvar BACKUP_PATH
. env.sh
 ## deploy Dgraph cluster and configure K8S CronJob with BACKUP_PATH
helmfile apply
```
#### Deploy Using Helm

```bash
## source script for envvar BACKUP_PATH
. env.sh
## deploy Dgraph cluster and configure K8S CronJob with BACKUP_PATH
helm repo add "dgraph" https://charts.dgraph.io
helm install "my-release" \
  --namespace default \
  --values ./charts/dgraph_config.yaml \
  --values ./charts/dgraph_secrets.yaml \
  --set backups.destination="${BACKUP_PATH}" \
  dgraph/dgraph
```

#### Access Resources

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

For Dgraph Ratel UI, you can use this to access it at http://localhost:8000:

```bash
export RATEL_POD_NAME=$(
 kubectl get pods \
  --namespace default \
  --selector "component=ratel,release=my-release" \
  --output jsonpath="{.items[0].metadata.name}"
)
kubectl --namespace default port-forward $RATEL_POD_NAME 8000:8000
```

#### Cleanup Kubernetes Environment

If you are using `helmfile`, you can delete the resources with:

```bash
## source script for envvar BACKUP_PATH
. env.sh
helmfile delete
kubectl delete pvc --selector release=my-release # release dgraph name specified in charts/helmfile.yaml
```

If you are just `helm`, you can delete the resources with:

```bash
helm delete my-release --namespace default "my-release" # dgraph release name used earlier
kubectl delete pvc --selector release=my-release # dgraph release name used earlier
```

## Triggering a Backup

This is run from the host with the alpha node accessible on localhost at port `8080`.  Can be done by running the docker-compose environment, or running `kubectl --namespace default port-forward pod/dgraph-dgraph-alpha-0 8080:8080`.

### Using GraphQL

For versions of Dgraph that support GraphQL, you can use this:

```bash
## source script for envvar BACKUP_PATH
. env.sh
## endpoint of alpha1 container
ALPHA_HOST="localhost"
## graphql mutation and required header
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
## source script for envvar BACKUP_PATH
. env.sh
## endpoint of alpha1 container
ALPHA_HOST="localhost"

curl --silent --request POST $ALPHA_HOST:8080/admin/backup?force_full=true --data "destination=$BACKUP_PATH"
```

This should return a response in JSON that will look like this if successful:

```JSON
{
  "code": "Success",
  "message": "Backup completed."
}
```
