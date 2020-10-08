# **AWS CloudFormation template**

> This is an AWS CloudFormation template to deploy a Dgraph cluster on AWS using EC2 instances in a separate VPC.

To deploy the cluster using the CloudFormation template we need two things set up:

1. SSH Keypair to assign to the created instances.
2. S3 bucket to store the applied CloudFormation templates.  A S3 bucket will be created for you if one does not exist.

Edit the `deploy.sh` file to change these variables to the configured values.

```sh
./deploy.sh <name-of-cloudformation-stack> <name-of-ssh-key-pair> [target-region] [s3-bucket-name]
```

Parameters:

* **name-of-cloudformation-stack**, e.g. `mydgraph-cluster`
* **name-of-key-pair**, e.g. `mydgraph-cluster-key`
* **target-region** (optional), e.g. `us-east-2`, region from default profile using `aws configure get region` will be used if not specified.
* **s3-bucket-name** (optional), e.g. `dgraph-marketplace-cf-stack-mydgraph-cluster-us-east-2`.  This will be created if not specified.


## **Notes**

### **Accessing Endpoint**

The security groups created will allow access from the Load Balancer. If you wish to access the endpoints from your public IP, you will need to edit the security group attached to the Load Balancer.  In the AWS web console, this can be found in the Description tab of the Load Balancer, from EC2 &rarr; Load Balancers &rarr; dgraph-load-balancer ( e.g. `xxxxx-Dgrap-XXXXXXXXXXXXX`).


You can also find the security group with this command:

```bash
MY_STACK_NAME=<name-of-cloudformation-stack>
MY_STACK_REGION=<target-region>

aws cloudformation describe-stack-resources \
--stack-name ${MY_STACK_NAME} \
--region ${MY_STACK_REGION} \
--logical-resource-id 'DgraphALBSecurityGroup' \
--query 'StackResources[0].PhysicalResourceId' \
--output text
```

In the Security field, there the `sg-xxxxxxxxxxxxxxxxx`, which you can click this link to get sent Security Groups, then edit the inbound rules for the same SG.  There should be existing inbound rules for ports `8000`, `8080`, `9080`.  Add new entries from your public IP to access those ports.

Also note the DNS information on Load Balancer description tab, like `xxxxx-Dgrap-XXXXXXXXXXXXX-1111111111.us-east-2.elb.amazonaws.com`, which you'll need to use to access the endpoint once access is enabled.  

Afterward, you can visit the website `http://xxxxx-Dgrap-XXXXXXXXXXXXX-1111111111.us-east-2.elb.amazonaws.com:8000`.  Once in the Dgraph Ratel UI, configure the server connection as: `http://xxxxx-Dgrap-XXXXXXXXXXXXX-1111111111.us-east-2.elb.amazonaws.com:8080`.

### **Accessing Systems with SSH**

If you need to access the EC2 instances themselves through SSH, update the security group on those instances. On any EC2 instance, edit the security group that looks like this `mydgraph-cluster-DgraphSecurityGroup-XXXXXXXXXXXX` and open up port 22 to your public IP.  Afterward, you can log into the system with something like this:

```bash
ssh -i /path/to/my-dgraph-cluster-key.pem ubuntu@ec2-X-XX-XXX-XXX.us-east-2.compute.amazonaws.com
```

### **ALB vs gRPC**

AWS ALBs (Application Load Balancers) configured with this template do not support gRPC load balancing. To get the best performance out of
the dgraph cluster, you can use an externally configured load balancer with gRPC capabilities like [HA Proxy](https://www.haproxy.com/blog/haproxy-1-9-2-adds-grpc-support/)
or [Nginx](https://www.nginx.com/blog/nginx-1-13-10-grpc/).

To know more about gRPC issues with AWS application load balancer, you can give [this blog](https://rokt.com/engineering_blog/learnings-grpc-aws/) a read.
