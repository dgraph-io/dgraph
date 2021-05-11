# Deploy Dgraph on AWS using Terraform

> **NOTE: This Terraform template creates a Dgraph database cluster with a public IP accessible to anyone. You can set the `assign_public_ip` variable
to false to skip creating a public IP address and you can configure access to Dgraph yourself.**

[Terraform](https://terraform.io/) automates the process spinning up the EC2 instance, setting up and running Dgraph in it.
This setup deploys terraform in standalone mode inside a single EC2 instance.

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

The output of `terraform apply` will contain the IP address assigned to your EC2 instance.

5. Use `terraform destroy` to delete the setup and restore the previous state.
