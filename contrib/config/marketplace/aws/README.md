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

The security groups created will allow access from the Load Balancer. If you wish to access the endpoints from your office, you will need to edit the security group attached to the Load Balancer.  In the AWS web console, this can be found in the Description tab of the Load Balancer, from EC2 &rarr; Load Balancers &rarr; dgraph-load-balancer ( e.g. `xxxxx-Dgrap-XXXXXXXXXXXXX`). In the Security field, there the `sg-xxxxxxxxxxxxxxxxx`, which you can click on and edit.

Also note the DNS for the LoadBalancer, like `xxxxx-Dgrap-XXXXXXXXXXXXX-1111111111.us-east-2.elb.amazonaws.com`, which you'll need to use to access the endpoint once access is enabled.  

### **Accessing Systems with SSH**

If you need to access the EC2 instances themselves through SSH, update the security group on those instances. On any EC2 instance, edit the security group that looks like this `mydgraph-cluster-DgraphSecurityGroup-XXXXXXXXXXXX` and open up port 22 to your office.  Afterward, you can log into the system with something like this:

```bash
ssh -i /path/to/my-dgraph-cluster-key.pem ubuntu@ec2-X-XX-XXX-XXX.us-east-2.compute.amazonaws.com
```

### **ALB vs gRPC**

AWS ALBs (Application Load Balancers) configured with this template do not support gRPC load balancing. To get the best performance out of
the dgraph cluster, you can use an externally configured load balancer with gRPC capabilities like [HA Proxy](https://www.haproxy.com/blog/haproxy-1-9-2-adds-grpc-support/)
or [Nginx](https://www.nginx.com/blog/nginx-1-13-10-grpc/).

To know more about gRPC issues with AWS application load balancer, you can give [this blog](https://rokt.com/engineering_blog/learnings-grpc-aws/) a read.
