# Deploy Dgraph on AWS using Terraform

[Terraform](https://terraform.io/) automates the process spinning up the EC2 instance, setting up and running Dgraph in it.
This setup deploys terraform in standalone mode inside a single EC2 instance.

Here are the steps to be followed:

1. You must have an AWS account set up.

2. [Download](https://terraform.io/downloads.html) and install terraform.

3. Create `terraform.tfvars` file similar to that of [terraform.tfvars.example](./terraform.tfvars.example) and edit the variables inside accordingly.
You can override any variable present in [variables.tf](./variables.tf) by providing an explicit value in `terraform.tfvars` file.
 
4. Execute the following commands:

```sh
$ terraform init
$ terraform plan
$ terraform apply
```

5. Use `terraform destroy` to delete the setup and restore the state.
