# Deploy Dgraph on AWS EKS using Terraform

Dgraph is a horizontally scalable and distributed graph database, providing ACID transactions,
consistent replication and linearizable reads. It's built from ground up to perform for a rich set
of queries. Being a native graph database, it tightly controls how the data is arranged on disk to
optimize for query performance and throughput, reducing disk seeks and network calls in a cluster.

### Introduction

The Terraform template creates the following resources towards setting up a Dgraph cluster on AWS EKS.

- AWS VPC with 2 private subnets for hosting the EKS cluster, 2 public subnets to host the load balancers to expose the services and one NAT subnet to provision the NAT gateway required for the nodes/pods in the private subnet to communicate with the internet. Also sets up the NACL rules for secure inter subnet communication.
- AWS EKS in the private subnets to host the Dgraph cluster.
- The Dgraph cluster Kubernetes resources in either a standalone mode or a HA mode(refer to the variables available to tweak the provisioning of the Dgraph cluster below) on the EKS cluster.

### Prerequisites

- Terraform > 0.12.0
- awscli >= 1.18.32

## Steps to follow to get the Dgraph cluster on AWS EKS up and running:

1. You must have an AWS account with privileges to create VPC, EKS and associated resources. Ensure awscli setup with the right credentials (One can also use AWS_PROFILE=\<profilename\> terraform \<command\> alternatively).

2. [Download](https://terraform.io/downloads.html) and install Terraform.

3. Create a `terraform.tfvars` file similar to that of `terraform.tfvars.example` and edit the variables inside accordingly.
   You can override any variable present in [variables.tf](./variables.tf) by providing an explicit value in `terraform.tfvars` file.

4. Execute the following commands:

```sh
$ terraform init
$ terraform plan -target=module.aws
$ terraform apply -target=module.aws
# One can choose to not run the following commands if they intend to use [Helm charts](https://github.com/dgraph-io/charts) to provision their resources on the Kubernetes cluster.
# If you want to manage the state of the Kubernetes resources using Terraform, run the following commands as well:
$ terraform plan -target=module.dgraph
$ terraform apply -target=module.dgraph
```

> Note: Both the modules cannot be applied in the same run owing to the way Terraform [evaluates](https://www.terraform.io/docs/providers/kubernetes/index.html#stacking-with-managed-kubernetes-cluster-resources) the provider blocks.

The command `terraform apply -target=module.dgraph` would output the hostnames of the Load Balancers exposing the Alpha, Zero and Ratel services. 

5. Use `terraform destroy -target=module.aws` to delete the setup and restore the previous state.



The following table lists the configurable parameters of the template and their default values:

| Parameter                 | Description                                                  | Default       |
| ------------------------- | ------------------------------------------------------------ | ------------- |
| `prefix`                  | The namespace prefix for all resources                       | dgraph        |
| `cidr`                    | The CIDR of the VPC                                          | 10.20.0.0/16  |
| `region`                  | The region to deploy the resources in                        | ap-south-1    |
| `ha`                      | Enable or disable HA deployment of Dgraph                    | true          |
| `ingress_whitelist_cidrs` | The CIDRs whitelisted at the service Load Balancer | ["0.0.0.0/0"] |
| `only_whitelist_local_ip` | "Only whitelist the IP of the executioner at the service Load Balancers | true          |
| `worker_nodes_count`      | The number of worker nodes to provision with the EKS cluster | 3             |
| `instance_types`		          | The list of instance types to run as worker nodes | ["m5.large"] |
| `namespace`               | The namespace to deploy the Dgraph pods to                   | dgraph        |
| `zero_replicas`           | The number of Zero replicas to create. Overridden by the ha variable which when disabled leads to creation of only 1 Zero pod | 3             |
| `zero_persistence`        | If enabled mounts a persistent disk to the Zero pods         | true          |
| `zero_storage_size_gb`       | The size of the persistent disk to attach to the Zero pods in GiB | 10            |
| `alpha_replicas`          | The number of Alpha replicas to create. Overridden by the ha variable which when disabled leads to creation of only 1 Alpha pod | 3             |
| `alpha_initialize_data`   | If set, runs an init container to help with loading the data into Alpha | false         |
| `alpha_persistence`       | If enabled, mounts a persistent disk to the Alpha pods        | true          |
| `alpha_storage_size_gb`      | The size of the persistent disk to attach to the Alpha pods in GiB | 10            |
| `alpha_lru_size_mb`               | The LRU cache to enable on Alpha pods in MiB                | 2048          |


> NOTE: 
>
> 1. If `ha` is set to `false` the `worker_node_count` is overridden to `1`.
>
> 2. If `only_whitelist_local_ip` is set to`true`, the `ingress_whitelist_cidrs is overridden` to local IP of the executioner.
>
> 3. The `kubeconfig` file is created in the root directory of this repository.
>
> 4. One could use Helm to install the Kubernetes resources onto the cluster, in which case comment out the `dgraph` module in `main.tf`.
>
> 5. The number of `worker_nodes` needs to be more than the greater of replicas of Zero/Alpha when `ha` is enabled to ensure the topological scheduling based on hostnames works.
> 
> 6. The hostnames of the service Load Balancers are part of the output of the run. Please use the respective service ports in conjunction with the hostnames. TLS is not enabled.
>
> 7. When `alpha_initialize_data`is set to `true`, an init container is provisioned to help with loading the data as follows:
>
>    ```
>          # Initializing the Alphas:
>          #
>          # You may want to initialize the Alphas with data before starting, e.g.
>          # with data from the Dgraph Bulk Loader: https://dgraph.io/docs/deploy/#bulk-loader.
>          # You can accomplish by uncommenting this initContainers config. This
>          # starts a container with the same /dgraph volume used by Alpha and runs
>          # before Alpha starts.
>          #
>          # You can copy your local p directory to the pod's /dgraph/p directory
>          # with this command:
>          #
>          #    kubectl cp path/to/p dgraph-alpha-0:/dgraph/ -c init-alpha
>          #    (repeat for each alpha pod)
>          #
>          # When you're finished initializing each Alpha data directory, you can signal
>          # it to terminate successfully by creating a /dgraph/doneinit file:
>          #
>          #    kubectl exec dgraph-alpha-0 -c init-alpha touch /dgraph/doneinit
>          #
>          # Note that pod restarts cause re-execution of Init Containers. If persistance is 	  # enabled /dgraph is persisted across pod restarts, the Init Container will exit
>          # automatically when /dgraph/doneinit is present and proceed with starting
>          # the Alpha process.
>    ```
