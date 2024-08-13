resource "aws_route_table" "route_public_subnets" {
  vpc_id = aws_vpc.vpc.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.ig.id
  }

  tags = {
    Name = "${var.cluster_name}-public-subnets-route-table"
  }
}

resource "aws_route_table" "route_private_subnets" {
  vpc_id = aws_vpc.vpc.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.nat_gw.id
  }

  tags = {
    Name = "${var.cluster_name}-private-subnets-route-table"
  }
}

# ASSOCIATIONS
##############

resource "aws_route_table_association" "lb_subnet_route_association" {
  count          = length(aws_subnet.lb_subnets)
  subnet_id      = aws_subnet.lb_subnets[count.index].id
  route_table_id = aws_route_table.route_public_subnets.id
}

resource "aws_route_table_association" "nat_subnet_route_association" {
  subnet_id      = aws_subnet.nat_subnet.id
  route_table_id = aws_route_table.route_public_subnets.id
}

resource "aws_route_table_association" "db_subnet_route_association" {
  count          = length(aws_subnet.db_subnets)
  subnet_id      = aws_subnet.db_subnets[count.index].id
  route_table_id = aws_route_table.route_private_subnets.id
}
