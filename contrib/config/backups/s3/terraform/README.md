# S3 Bucket with Terraform

## About

This script will create the required resources needed to create S3 (Simple Storage Service) bucket using [`s3-bucket`](github.com/darkn3rd/s3-bucket) module.

## Prerequisites

You need the following installed to use this automation:

* [AWS CLI](https://aws.amazon.com/cli/) - AWS CLI installed and configured with local profile
* [Terraform](https://www.terraform.io/downloads.html) - tool used to provision resources and create templates

## Configuration

You will need to define the following variables:

* Required Variables:
  * `region` (required) - region where bucket will be created
  * `name` (required) - unique name of s3 bucket

## Steps

### Define Variables

You can define these when prompted, or in `terrafrom.tfvars` file, or through command line variables, e.g. `TF_VAR_name`, `TF_VAR_region`.

```terraform
# terraform.tfvars
name   = "my-organization-backups"
region = "us-west-2"
```

### Download Plugins and Modules

```bash
terraform init
```

### Prepare and Provision Resources

This will create an S3 bucket and an IAM user that has access to that bucket.  For convenience, will also generate the following files:

* `../s3.env` - used to demonstrate or test dgraph backups with s3 bucket in local docker environment
* `../env.sh`- destination string to use trigger backups from the command line or to configure Kubernetes cron jobs to schedule backups
* `../charts/dgraph_secrets.yaml` - used to deploy Dgraph with support for backups

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
