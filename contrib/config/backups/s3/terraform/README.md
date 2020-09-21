# Azure Blob with Terraform

## About

This script will create the required resources needed to create Simple Storage Service bucket using [`s3-bucket`](github.com/darkn3rd/s3-bucket) module.

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
