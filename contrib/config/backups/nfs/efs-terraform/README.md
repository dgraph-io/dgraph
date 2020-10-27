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


* **Required**
 * `vpc_name` or `vpc_id` - specify either explicit `vpc_id` or a name of Tag `Name` used
 * `subnets` or use [discovery](#discovery) - specify Subnet IDs for subnets that will have access to EFS, or have this discovered automatically
* **Optional**
 * `security_groups` or use [discovery](#discovery) - specify SG IDs of security groups to add that will allow access to EFS server, or have this discovered automatically.
 * `dns_name` with `dns_domain` or `zone_id` - this is used to create a friendly alternative name such as `myfileserver.devest.mycompany.com`
 * `encrypted` (default: false) - whether EFS storage is encrypted or not

## Discovery

The following are configured to discover resources used to configure EFS.  These can be overriden by specifying values as input variables.

These are values affected by discovery:

  * **VPC Name** - you can supply either explicit `vpc_id` or `vpc_name` if VPC has the tag `Name`.
  * **EKS Cluster Name** - if `eks_cluster_name` is not specified, then the VPC tag `Name` will be used as the EKS Cluster Name.  This is default configuration if both VPC and EKS cluster was provisioned by `eksctl`.
  * **Private Subnets** - if `subnets` is not specified, private subnets used by an EKS cluster can be discovered provided the tags are set up appropriately (see [Requirements for Discovery](#requirements-for-discovery))
  * **Security Group** (optional for access)- if `security_groups` is not specified this security group can be discovered provided the tags are set up appropriately (see [Requirements for Discovery](#requirements-for-discovery))
  * **DNS Domain** (optional for DNS name)- a domain name, e.g. `devtest.mycompany.com`, managed by Route53 can be specified to fetch a Zone ID, otherwise a `zone_id` must be specified to use this feature.  When using this, you need to supply the name you want to use, e.g. `myfileserver` with `dns_name`

### Requirements for Discovery

For the discovery feature where this Terraform script will find resources needed that will allow EFS configured alongside and acessed by Amazon EKS cluster, you will need to have the appropriate tags per resources.  If you used `eksctl` to provision your cluster, these tags and keys will be setup automatically.

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

If discovery is setup (see [Requirements for Discovery](#requirements-for-discovery)), you can specify this for `terraform.tfvars` files:

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

When finished you can destroy resources created with Terraform using this:

```bash
terraform destroy
```
