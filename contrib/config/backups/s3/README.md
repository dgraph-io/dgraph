# Binary backups to S3

Binary backups can use AWS S3 (Simple Storage Service) for an object storage.

## Provisioning S3

Some example scripts have been provided to illustrate how to create S3.

* [Terraform](terraform/README.md) - terraform scripts to provision S3 bucket and an IAM user with access to the S3 bucket.

## Setting up the environment

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

You will need to create an `s3.env` file first like the example below. If you created the S3 bucket using the [Terraform](terraform/README.md) scripts, this will have been created automatically.

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

#### Clean up the Docker Environment

```bash
docker-compose stop
docker-compose rm
```

### Using Kubernetes with Helm Charts

For Kubernetes, you can deploy a Dgraph cluster and a Kubernetes Cronjob that triggers backups using [Helm](https://helm.sh/docs/intro/install/).

#### Configuring secrets values

These values are automatically created if you used the [Terraform](terraform/README.md) scripts.  

If you already an existing S3 bucket you would like to use, you will need to create `charts/dgraph_secrets.yaml` files as shown below.  Otherwise, if you created the bucket using the [Terraform](terraform/README.md) scripts, then this would be created automatically.

For the `charts/dgraph_secrets.yaml`, you would create a file like this:

```yaml
backups:
  keys:
    s3:
      ## AWS_ACCESS_KEY_ID
      access: <aws-access-key>
      ## AWS_SECRET_ACCESS_KEY
      secret: <aws-secret-key>
```

#### Configuring Environments

We need to define one environment variable `BACKUP_PATH`.  If [Terraform](terraform/README.md) scripts were used to create the S3 bucket, we can source the `env.sh` or otherwise create it here:

```bash
## env.sh
export BACKUP_PATH=s3://s3.<region>.amazonaws.com/<s3-bucket-name>
```

#### Deploy using Helmfile

If you have [helmfile](https://github.com/roboll/helmfile#installation) and the [helm-diff](https://github.com/databus23/helm-diff) plugin installed, you can deploy a Dgraph cluster with the following:

```bash
## source script for BACKUP_PATH env var 
. env.sh
 ## deploy Dgraph cluster and configure K8S CronJob with BACKUP_PATH
helmfile apply
```
#### Deploy using Helm

```bash
## source script for BACKUP_PATH env var
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

#### Access resources

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

#### Cleanup the Kubernetes environment

If you are using `helmfile`, you can delete the resources with:

```bash
## source script for BACKUP_PATH env var
. env.sh
helmfile delete
kubectl delete pvc --selector release=my-release # release dgraph name specified in charts/helmfile.yaml
```

If you are just `helm`, you can delete the resources with:

```bash
helm delete my-release --namespace default "my-release" # dgraph release name used earlier
kubectl delete pvc --selector release=my-release # dgraph release name used earlier
```

## Triggering a backup

This is run from the host with the alpha node accessible on localhost at port `8080`.  This can can be done by running the `docker-compose` environment, or in the Kubernetes environment, after running `kubectl --namespace default port-forward pod/dgraph-dgraph-alpha-0 8080:8080`.

### Using GraphQL

For versions of Dgraph that support GraphQL, you can use this:

```bash
## source script for BACKUP_PATH env var
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
## source script for BACKUP_PATH env var
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
