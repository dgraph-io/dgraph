# Binary Backups to Azure Blob

Binary backups can use Azure Blob Storage oject storage using MinIO Azure Gateway.

## Provisioning Azure Blob

Some example scripts have been provided to illustrate how to create Azure Blob.

* [azure_cli](azure_cli/README.md) - shell scripts to provision Azure Blob and create `minio.env`
* [terraform](terraform/README.md) - terraform scripts to provision Azure Blob and create `minio.env`

## Setting up the Environment

### Prerequisites

You will need these tools:

* [Docker](https://docs.docker.com/get-docker/)
* [Docker Compose](https://docs.docker.com/compose/install/)

### Using Docker Compose

A `docker-compose.yml` configuration is provided that will run the Azure gateway and Dgraph cluster.

### Configuring Docker Compose

You will need to create a `minio.env` first:

```bash
MINIO_ACCESS_KEY=<Azure Storage Account Name>
MINIO_SECRET_KEY=<Azure Storage Account Key>
```

These values are used to both access the Minio Gateway using the same credentials used to access Azure Storage Account.  Both example terraform and Azure CLI scripts can auto-generate the `minio.env`

### Using Docker Compose

```bash
## Run Minio Azure Gateway and Dgraph Cluster
docker-compose up --detach
```

### Access Services

* Minio UI: http://localhost:9000
* Ratel UI: http://localhost:8000

### Triggering a Backup using GraphQL

```bash
ALPHA_HOST="localhost"  # hostname to connect to alpha1 container
MINIO_HOST="gateway"    # hostname from alpha1 container
BLOB_NAME=dgraph-backups # blob container name
BACKUP_PATH=minio://${MINIO_HOST}:9000/${BLOB_NAME}?secure=false
GQL="{\"query\": \"mutation { backup(input: {destination: \\\"$BACKUP_PATH\\\" forceFull: true}) { response { message code } } }\"}"
HEADER="Content-Type: application/json"
curl --silent --header "$HEADER" --request POST $ALPHA_HOST:8080/admin --data "$GQL"
```

This should return back response in JSON that will look like this if successful:

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
