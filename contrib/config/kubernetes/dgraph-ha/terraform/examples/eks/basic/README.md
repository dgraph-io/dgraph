# Basic EKS Cluster Example

## Prerequisites

* [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/install-cliv1.html)
* [Kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
* [Helm 3](https://helm.sh/)
* [Terraform](https://www.terraform.io/downloads.html)

## Instructions

### Provision Kubernetes Cluster (EKS)

```bash
## configure Terraform input variables
export TF_VAR_eks_cluster_name="dgraph-tf-cluster"
export TF_VAR_region="us-east-2"

## download modules and providers
terraform init
## provision Kubernetes infrastructure
terraform apply
```

### Configure Kubeconfig and Test Cluster

```bash
## add Kubenetes credentials
aws eks --region $TF_VAR_region update-kubeconfig --name $TF_VAR_eks_cluster_name
## test cluster
kubectl get all --all-namespaces
```

### Deploy Dgraph

```bash
## install dgraph
export MY_DGRAPH_RELEASE="my-release"
helm repo add dgraph https://charts.dgraph.io
helm install $MY_DGRAPH_RELEASE --set image.tag="latest" dgraph/dgraph
```

### Remove Dgraph

```bash
helm uninstall $MY_DGRAPH_RELEASE
kubectl delete pvc --selector release=$MY_DGRAPH_RELEASE
```

### Remove Kubernetes Cluster

```bash
terraform destroy

# remove KUBECONFIG entry
TARGET=$(kubectl config get-contexts | awk "/$TF_VAR_eks_cluster_name/{ print \$2 }")
kubectl config delete-context $TARGET
kubectl config delete-cluster $TARGET

```

## References

* Dgraph Deploy Documentation
  * [HA Cluster Setup Using Kubernetes](https://dgraph.io/docs/master/deploy/#ha-cluster-setup-using-kubernetes)
  * [Using Helm Chart](https://dgraph.io/docs/master/deploy/#using-helm-chart)
* Dgraph Helm Chart Source Code: https://github.com/dgraph-io/charts/
