##
## Variables
variable region {}
variable eks_cluster_name {}

##
## eks-cluster module
module "eks-cluster" {
  source           = "../../../modules/eks/"
  region           = var.region
  eks_cluster_name = var.eks_cluster_name
}
