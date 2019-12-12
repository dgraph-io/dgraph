# Deploy Dgraph on AWS using Terraform

[Terraform](https://terraform.io/) automates the process spinning up the EC2 instance, setting up and running Dgraph in it.
This setup deploys terraform in standalone mode inside a single EC2 instance.

Here are the steps to follow:

1. You must have an AWS account set up.

2. [Download](https://terraform.io/downloads.html) and install terraform.

3. Create `terraform.tfvars` file similar to that of [terraform.tfvars.example](./terraform.tfvars.example) and edit the variables inside accordingly.
You can override any variable present in [variables.tf](./variables.tf) by providing an explicit value in a `terraform.tfvars` file.
 
4. Execute the following commands:

```sh
$ terraform init
$ terraform plan
$ terraform apply
```

The output of `terraform apply` will contain the IP address assigned to your EC2 instance. Dgraph-ratel will be available on `<OUTPUT_IP>:8000`.
Change the server URL in the dashboard to `<OUTPUT_IP>:8080` and start playing with dgraph.

5. Use `terraform destroy` to delete the setup and restore the previous state.
