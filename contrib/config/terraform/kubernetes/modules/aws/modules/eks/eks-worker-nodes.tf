resource "aws_iam_role" "worker_role" {
  name = "${var.cluster_name}-worker-role"

  assume_role_policy = <<POLICY
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "ec2.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
POLICY
}


resource "aws_iam_role_policy_attachment" "eks_worker_nodepolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
  role       = aws_iam_role.worker_role.name
}


resource "aws_iam_role_policy_attachment" "eks_cnipolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
  role       = aws_iam_role.worker_role.name
}


resource "aws_iam_role_policy_attachment" "container_readonlypolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
  role       = aws_iam_role.worker_role.name
}


resource "aws_eks_node_group" "nodegroup" {
  cluster_name = aws_eks_cluster.eks.name

  instance_types  = var.instance_types
  node_group_name = "${var.cluster_name}-nodegroup"
  node_role_arn   = aws_iam_role.worker_role.arn
  subnet_ids      = var.db_subnet_ids

  scaling_config {
    desired_size = var.ha ? var.worker_nodes_count : 1
    max_size     = var.ha ? var.worker_nodes_count : 1
    min_size     = var.ha ? var.worker_nodes_count : 1
  }

  tags = map(
    "Name", "${var.cluster_name}-nodegroup",
    "kubernetes.io/cluster/${var.cluster_name}", "owned",
  )

  depends_on = [
    aws_iam_role_policy_attachment.eks_worker_nodepolicy,
    aws_iam_role_policy_attachment.eks_cnipolicy,
    aws_iam_role_policy_attachment.container_readonlypolicy,
  ]
}
