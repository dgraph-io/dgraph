# Amazon Elastic File Services with Terraform

This script will create the required resources needed to create NFS server instance using [Amazon Elastic File Services](https://aws.amazon.com/efs/).

This script will create the following resources:

* EFS Server
* SG to allow EKS worker nodes to access EFS Server (if discovery used)
* Conifguration file (`../env.sh`) that specifies NFS Server and Path

## Prerequisites

You need the following installed to use this automation:

* [AWS CLI](https://aws.amazon.com/cli/) - AWS CLI installed and configured with local profile
* [Terraform](https://www.terraform.io/downloads.html) - tool used to provision resources and create templates

## Configuration

* Either VPC ID or tag `Name` of VPC is required
* Details on eks_cluster_name if differs from VPC name (eksctl style)

## Discovery

TBA

* vpcid
* subnets (private)
* security group
* route53

### Requirements for Discovery

TBA

* SGs for EKS Worker Nodes must be tagged with a similar schema that `eksctl` uses

## Steps

### Define Variables

TBA

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

When finished you can destroy resources created with Terraform using this:

```bash
terraform destroy
```
