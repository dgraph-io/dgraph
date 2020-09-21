# Google Cloud Storage with Terraform

## About

This script will create the required resources needed to create a bucket in Google Storage Bucket using the (`simple-bucket`)[https://github.com/terraform-google-modules/terraform-google-cloud-storage/tree/master/modules/simple_bucket] Terraform module.  These scripts will also create a `credentials.json` that will have access to the storage bucket, which is needed for the [MinIO GCS Gateway](https://docs.min.io/docs/minio-gateway-for-gcs.html) and optionally generate random MinIO access key and secret key.

## Prerequisites

You need the following installed to use this automation:

* [Google Cloud SDK](https://cloud.google.com/sdk/docs/install) - for the `gcloud` commmand and required to access Google Cloud.
   * Google Project with billing enabled
   * `gcloud` logged into IAM account with roles added:
      * `serviceusage.apiKeys.create`
      * `clientauthconfig.clients.create`
      * `iam.serviceAccountKeys.create`
* [Terraform](https://www.terraform.io/downloads.html) - tool used to provision resources and create templates

## Configuration

You will need to define the following variables:

* Required Variables:
  * `region` (required) - region where the GCS bucket will be created
  * `project_id` (required) - gobally unique name for the Google project that will contain the GCS bucket
  * `name` (default = `my-dgraph-backups`) - globally unique name of the GCS bucket
* Optional Variables:
  * `minio_access_key` - specify access key or have terraform generate a random access key
  * `minio_secret_key` - specify secret key or have terraform generate a random secret key

## Steps

### Define Variables

You can define these when prompted, or in `terrafrom.tfvars` file, or through command line variables, e.g. `TF_VAR_project_id`, `TF_VAR_project_id`, and `TF_VAR_name`. Below is an example `terraform.tfvars` file:

```terraform
# terraform.tfvars
region     = "us-central1"
project_id = "the-alan-parsons"
name       = "my-backups-31393832"
```

### Download Plugins and Modules

```bash
terraform init
```

### Prepare and Provision Resources

```bash
## get a list of changes that will be made
terraform plan
## apply the changes
terraform apply
```

## Cleanup

```bash
terraform destroy
```
