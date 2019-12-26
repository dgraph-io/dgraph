# Deploy Dgraph on AWS using Terraform
[Terraform](https://terraform.io/) automates the process spinning up the EC2 instance, setting up and running Dgraph in it.

Here are the steps to be followed,
1. Have an AWS account.

2. [Download terraform](https://terraform.io/downloads.html) and install terraform.

3. Add your AWS secret key and access key in `creds/aws_secrets` file.

4. Create an aws EC2 [key pair](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-key-pairs.html) to connect to remote ec2 instance. Download the `.pem` file and save it as `aws.pem` inside the `creds` folder.

5. By default the instance type `t2.micro`. Edit the `variable instance_type` entry in `variables.tf` to change the instance type.
 
6. Execute the following commands.
```sh
$ terraform init
$ terraform plan
$ terraform apply
````

7. Use `terraform destroy` to delete the setup and restore the state.
