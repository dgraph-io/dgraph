# Highly Available Dgraph on AWS using terraform

[Terraform](https://terraform.io/) automates the process of spinning up the EC2 instance, setting up, and running Dgraph in it.
This setup deploys terraform in HA mode in AWS.

Here are the steps to follow:

1. You must have an AWS account set up.

2. [Download](https://terraform.io/downloads.html) and install terraform.

3. Create a `terraform.tfvars` file similar to that of [terraform.tfvars.example](./terraform.tfvars.example) and edit the variables inside accordingly.
You can override any variable present in [variables.tf](./variables.tf) by providing an explicit value in `terraform.tfvars` file.

4. Execute the following commands:

```sh
$ terraform init
$ terraform plan
$ terraform apply
```

The output of `terraform apply` will contain the Load Balancer DNS name configured with the setup.

5. Use `terraform destroy` to delete the setup and restore the previous state.

### Note

* The terraform setup has been tested to work well with AWS [m5](https://aws.amazon.com/ec2/instance-types/m5/) instances.

* AWS ALBs (Application Load Balancers) configured with this template do not support gRPC load balancing. To get the best performance out of the Dgraph cluster, you can use an externally configured load balancer with gRPC capabilities like [HA Proxy](https://www.haproxy.com/blog/haproxy-1-9-2-adds-grpc-support/) or [Nginx](https://www.nginx.com/blog/nginx-1-13-10-grpc/).
