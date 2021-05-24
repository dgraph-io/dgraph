# Deploy Dgraph on GCP using Terraform

> **NOTE: This Terraform template creates a Dgraph database cluster with a public IP accessible to anyone. You can set the `assign_public_ip` variable
to false to skip creating a public IP address and you can configure access to Dgraph yourself.**

[Terraform](https://terraform.io/) automates the process spinning up GCP compute instance, setting up and running Dgraph in it.
This setup deploys terraform in standalone mode inside a single GCP compute instance.

Here are the steps to be followed:

1. You must have a GCP account set up.

2. [Download](https://terraform.io/downloads.html) and install terraform.

3. Generate service account keys for your GCP account either using the dashboard or `gcloud` CLI as shown below:

```sh
gcloud iam service-accounts keys create ./account.json \
  --iam-account [SA-NAME]@[PROJECT-ID].iam.gserviceaccount.com
```

4. Execute the following commands:

```sh
$ terraform init

$ TF_VAR_project_name=<GCP Project Name> terraform plan

$ terraform apply

Apply complete! Resources: 1 added, 0 changed, 0 destroyed.

Outputs:

dgraph_ip = <OUTPUT_IP>
```

The output of `terraform apply` will contain the IP address assigned to your instance.

5. Use `terraform destroy` to delete the setup and restore the state.
