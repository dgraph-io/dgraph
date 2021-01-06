# Google Cloud Filestore using Google Cloud SDK (Shell)

This shell script creates the resources needed to create an NFS server instance using Google Cloud Filestore.

This automation will create the following resources:

  * [Google Cloud Filestore Server](https://cloud.google.com/filestore)
  * Configuration file (`../env.sh`) that specifies NFS Server and Path

## Prerequisites

You need the following installed to use this automation:

* [Google Cloud SDK](https://cloud.google.com/sdk/docs/install) - for the `gcloud` command and required to access Google Cloud.
* [bash](https://www.gnu.org/software/bash/) - shell environment

## Configuration

You will need to define these environment variables:

* Required Variables:
  * `MY_FS_NAME` (required) - Name of Filestore instance.
* Optional Variables:
  * `MY_PROJECT` (default to current configured project) - Project with billing enabled to create Filestore instance.
  * `MY_ZONE` (default `us-central1-b`) - zone where Filestore instance will be created
  * `MY_FS_CAPACITY` (default `1TB`) - size of the storage used for Filestore
  * `MY_FS_SHARE_NAME` (default `volumes`) - NFS path

## Create Filestore

Run these steps to create [filestore](https://cloud.google.com/filestore) and populate the configuration (`../env.sh`)

### Define Variables

You can create an `env.sh` with the desired values, for example:

```bash
cat <<-EOF > env.sh
export MY_FS_NAME="my-organization-nfs-server"
export MY_PROJECT="my-organization-test"
export MY_ZONE="us-central1-b"
EOF
```

These values can be used to create and destroy [filestore](https://cloud.google.com/filestore).

### Run the Script

```bash
## get env vars used to create filestore
. env.sh
## create filestore and populate ../env.sh
./create_gcfs.sh <filestore-name>
```

## Cleanup

You can run these commands to delete the resources (with prompts) on GCP.

```bash
## get env vars used to create filestore
. env.sh

## conditionally delete filestore if it exists (idempotent)
if gcloud filestore instances list | grep -q ${MY_FS_NAME}; then
  gcloud filestore instances delete ${MY_FS_NAME} \
    --project=${MY_PROJECT} \
    --zone=${MY_ZONE}
fi

## remove configuration that points to deleted filestore
rm ../env.sh
```
