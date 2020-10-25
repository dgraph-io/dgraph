# EFS

This creates the following:

* Amazon EFS Server
* SGs that allow EKS worker nodes to reach EFS Server
* dgraph.yaml helm chart values file

## Requirements

* Infrastructure
  * Either VPC ID or tag `Name` of VPC are required
  * SGs for EKS Worker Nodes must be tagged with similar schema that `eksctl` uses
