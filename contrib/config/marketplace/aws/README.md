# AWS CloudFormation template

> This is an AWS CloudFormation template to deploy a Dgraph cluster on AWS using EC2 instances in a separate VPC.

To deploy the cluster using the CloudFormation template we need two things set up:

1. SSH Keypair to assign to the created instances.
2. S3 bucket to store the applied CloudFormation templates.

Edit the `deploy.sh` file to change these variables to the configured values.

```sh
$ cat deploy.sh
...
readonly ssh_key_name="dgraph-cloudformation-deployment-key"
readonly s3_bucket_name="dgraph-marketplace-cf-template-${region}"
...

$ ./deploy.sh <name-of-cloudformation-stack>
```
