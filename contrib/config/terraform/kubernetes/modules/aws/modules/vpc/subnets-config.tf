resource "aws_subnet" "lb_subnets" {
  count = 2

  vpc_id            = aws_vpc.vpc.id
  cidr_block        = cidrsubnet(cidrsubnet(var.cidr, 1, 0), 2, count.index)
  availability_zone = data.aws_availability_zones.az.names[count.index]

  map_public_ip_on_launch = true

  tags = map(
    "Name", "${var.cluster_name}-lb-az-${count.index}",
    "kubernetes.io/cluster/${var.cluster_name}", "shared",
    "kubernetes.io/role/elb", "1",
  )
}


resource "aws_subnet" "nat_subnet" {
  vpc_id                  = aws_vpc.vpc.id
  cidr_block              = cidrsubnet(cidrsubnet(var.cidr, 1, 0), 2, 2)
  availability_zone       = data.aws_availability_zones.az.names[0]
  map_public_ip_on_launch = false

  tags = map(
    "Name", "${var.cluster_name}-nat",
  )
}


resource "aws_subnet" "db_subnets" {
  count = 2

  vpc_id            = aws_vpc.vpc.id
  cidr_block        = cidrsubnet(cidrsubnet(var.cidr, 1, 1), 1, count.index)
  availability_zone = data.aws_availability_zones.az.names[count.index]

  map_public_ip_on_launch = false

  tags = map(
    "Name", "${var.cluster_name}-db-az-${count.index}",
    "kubernetes.io/cluster/${var.cluster_name}", "shared",
    "kubernetes.io/role/internal-elb", "1",
  )
}
