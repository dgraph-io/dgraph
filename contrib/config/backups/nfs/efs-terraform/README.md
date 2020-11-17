# Amazon Elastic File Services with Terraform

These [Terraform](https://www.terraform.io/) scripts and modules will create the resources required to support an NFS server instance using [Amazon Elastic File Services](https://aws.amazon.com/efs/).

This automation script will create the following resources:

* [EFS](https://aws.amazon.com/efs/) Server
* SG to allow EKS worker nodes to access the [EFS](https://aws.amazon.com/efs/) Server (if discovery used)
* Configuration file (`../env.sh`) that specifies NFS Server and Path

## Prerequisites

To use this automation, you must install the following:

* [AWS CLI](https://aws.amazon.com/cli/) - AWS CLI installed and configured with local profile
* [Terraform](https://www.terraform.io/downloads.html) - tool used to provision resources and create templates

## Configuration

You can use the following input variables to configure this automation:

* **Required**
 * `vpc_name` or `vpc_id` - specify either explicit `vpc_id` or a name of Tag `Name` used
 * `subnets` or use [discovery](#discovery) - specify Subnet IDs for subnets that will have access to EFS, or have this discovered automatically
* **Optional**
 * `security_groups` or use [discovery](#discovery) - specify SG IDs of security groups to add that will allow access to EFS server, or have this discovered automatically.
 * `dns_name` with `dns_domain` or `zone_id` - this is used to create a friendly alternative name such as `myfileserver.devest.mycompany.com`
 * `encrypted` (default: false) - whether EFS storage is encrypted or not

## Discovery

Configuring the following values allows this automation to discover the resources used to configure EFS. These can be overridden by specifying explicit values as input variables.

These are values affected by discovery:

  * **VPC Name** - you can supply either explicit `vpc_id` or `vpc_name` if VPC has a tag key of `Name`.
  * **EKS Cluster Name** - if `eks_cluster_name` is not specified, then the VPC tag `Name` will be used as the EKS Cluster Name.  This is default configuration if both VPC and EKS cluster that was provisioned by `eksctl`.
  * **Private Subnets** - if `subnets` is not specified, private subnets used by an EKS cluster can be discovered provided that the tags are set up appropriately (see [Requirements for Discovery](#requirements-for-discovery))
  * **Security Group** (optional for access)- if `security_groups` is not specified this security group can be discovered provided that the tags are set up appropriately (see [Requirements for Discovery](#requirements-for-discovery))
  * **DNS Domain** (optional for DNS name)- a domain name, e.g. `devtest.mycompany.com.`, managed by Route53 can be specified to fetch a Zone ID, otherwise a `zone_id` must be specified to use this feature.  When using this, you need to supply the CNAME you want to use, e.g. `myfileserver` with `dns_name`

### Requirements for Discovery

You will need to have the appropriate tags per subnets and security groups configured to support the discovery feature. This feature will allow these [Terraform](https://www.terraform.io/) scripts to find the resources required to allow EFS configuration alongside an Amazon EKS cluster and SG configuration to allow EKS worker nodes to access EFS.  If you used `eksctl` to provision your cluster, these tags and keys will be set up automatically.

#### Subnets

Your private subnets where EKS is installed should have the following tags:

| Tag Key                                     | Tag Value |
|---------------------------------------------|-----------|
| `kubernetes.io/cluster/${EKS_CLUSTER_NAME}` | `shared`  |
| `kubernetes.io/role/internal-elb`           | `1`       |

#### Security Groups

A security group used to allow access to EKS Nodes needs to have the following tags:

| Tag Key                                     | Tag Value            |
|---------------------------------------------|----------------------|
| `kubernetes.io/cluster/${EKS_CLUSTER_NAME}` | `owned`              |
| `aws:eks:cluster-name`                      | `{EKS_CLUSTER_NAME}` |

## Steps

### Define Variables

If discovery was configured (see [Requirements for Discovery](#requirements-for-discovery)), you can specify this for `terraform.tfvars` files:

```hcl
vpc_name         = "dgraph-eks-test-cluster"
region           = "us-east-2"

## optional DNS values
dns_name         = "myfileserver"
dns_domain       = "devtest.example.com."
```

Alternatively, you can supply the SG IDs and Subnet IDs explicitly in `terraform.tfvars`:

```hcl
vpc_id           = "vpc-xxxxxxxxxxxxxxxxx"
eks_cluster_name = "dgraph-eks-test-cluster"
region           = "us-east-2"

## optional DNS values
dns_name         = "myfileserver"
zone_id          = "XXXXXXXXXXXXXXXXXXXX"

## Specify subnets and security groups explicitly
subnets = [
 "subnet-xxxxxxxxxxxxxxxxx",
 "subnet-xxxxxxxxxxxxxxxxx",
 "subnet-xxxxxxxxxxxxxxxxx",
]

security_groups = [
  "sg-xxxxxxxxxxxxxxxxx",
]
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
