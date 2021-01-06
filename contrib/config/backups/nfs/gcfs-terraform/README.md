# Google Cloud Filestore with Terraform

These [Terraform](https://www.terraform.io/) scripts and modules will create the resources required to create an NFS server instance using Google Cloud Filestore.

This automation will create the following resources:

  * [Google Cloud Filestore Server](https://cloud.google.com/filestore)
  * Configuration file (`../env.sh`) that specifies NFS Server and Path

## Prerequisites

You need the following installed to use this automation:

* [Google Cloud SDK](https://cloud.google.com/sdk/docs/install) - for the `gcloud` command and required to access Google Cloud.
   * Google Project with billing enabled
* [Terraform](https://www.terraform.io/downloads.html) - tool used to provision resources and create templates

## Configuration

You will need to define the following variables:

* Required Variables:
  * `project_id` (required) - a globally unique name for the Google project that will contain the GCS bucket
  * `name` (required) - name of GCFS server instance
* Optional Variables:
  * `zone` (default = `us-central1-b`) - specify zone where instances will be located
  * `tier` (default = `STANDARD`) - service tier of the instance, e.g. `TIER_UNSPECIFIED`, `STANDARD`, `PREMIUM`, `BASIC_HDD`, `BASIC_SSD`, and `HIGH_SCALE_SSD`.
  * `network` (default = `default`) - specify a GCE VPC network to which the instance is connected.
  * `capacity_gb` (default = `1024`) - specify file share capacity in GiB (minimum of `1024`)
  * `share_name` (default = `volumes`)- specify a name of the file share

## Steps

### Define Variables

You can define these when prompted, in `terrafrom.tfvars` file, or through command line variables, e.g. `TF_VAR_project_id`, `TF_VAR_project_id`, and `TF_VAR_name`. Below is an example `terraform.tfvars` file:

```terraform
## terraform.tfvars
name       = "my-company-nfs-backups"
project_id = "my-company-test"
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

When finished, you can destroy resources created with [Terraform](https://www.terraform.io/) using this:

```bash
terraform destroy
```
