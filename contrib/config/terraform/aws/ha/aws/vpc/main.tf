# The architecture of the VPC is as follows:
# * Primary Subnet - Private subnet where everything is deployed.
# * Secondary Subnet - Public subnet from where we route things to the internet.
#
# In the primary subnet we deploy all the application that dgraph is concerned with, since
# this subnet is private these instances cannot be accessed outside the VPC.
#
# Primary subnet contains a route table which contains an entry to route all the traffic
# destining to 0.0.0.0/0 via the nat gateway we have configured. This is so as to allow
# access from inside the the instance to the outside world.
#
# The nat instance gateway and the internet gateway are then deployed in the other subnet
# which is public. The route table entry of this subnet routes all the traffic destined
# to 0.0.0.0/0 via internet gateway so that it is accessible.
#
# A typical outbound connection from dgraph instance to google.com looks something like this
# Instance --> Route --> NAT Instance(in public subnet) --> Route --> Internet Gateway(in public subnet)
resource "aws_vpc" "dgraph" {
  cidr_block         = var.cidr_block
  enable_dns_support = true
  instance_tenancy   = "dedicated"

  # For enabling assignment of private dns addresses within AWS.
  enable_dns_hostnames = true

  tags = {
    Name = var.name
  }
}

resource "aws_eip" "dgraph_nat" {
  vpc = true
}

resource "aws_internet_gateway" "dgraph_gw" {
  vpc_id = aws_vpc.dgraph.id

  tags = {
    Name = var.name
  }
}

resource "aws_nat_gateway" "dgraph_gw" {
  allocation_id = aws_eip.dgraph_nat.id
  subnet_id     = aws_subnet.dgraph_secondary.id

  tags = {
    Name = var.name
  }
}

resource "aws_route_table" "dgraph_igw" {
  vpc_id = aws_vpc.dgraph.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.dgraph_gw.id
  }

  tags = {
    Name = var.name
  }
}

resource "aws_route_table_association" "internet_gw" {
  subnet_id      = aws_subnet.dgraph_secondary.id
  route_table_id = aws_route_table.dgraph_igw.id
}

resource "aws_main_route_table_association" "dgraph" {
  vpc_id         = aws_vpc.dgraph.id
  route_table_id = aws_route_table.dgraph_igw.id
}

resource "aws_route_table" "dgraph_ngw" {
  vpc_id = aws_vpc.dgraph.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.dgraph_gw.id
  }

  tags = {
    Name = var.name
  }
}

resource "aws_route_table_association" "nat_gw" {
  subnet_id      = aws_subnet.dgraph.id
  route_table_id = aws_route_table.dgraph_ngw.id
}

resource "aws_subnet" "dgraph" {
  vpc_id     = aws_vpc.dgraph.id
  cidr_block = var.subnet_cidr_block

  availability_zone_id = data.aws_availability_zones.az.zone_ids[0]

  tags = {
    Name = var.name
  }
}

resource "aws_subnet" "dgraph_secondary" {
  vpc_id     = aws_vpc.dgraph.id
  cidr_block = var.secondary_subnet_cidr_block

  availability_zone_id = data.aws_availability_zones.az.zone_ids[1]

  tags = {
    Name = var.name
    Type = "secondary-subnet"
  }
}

resource "aws_security_group" "dgraph_client" {
  name        = "dgraph-cluster-client"
  description = "Security group that can be used by the client to connect to the dgraph alpha instance using ALB."
  vpc_id      = aws_vpc.dgraph.id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [var.cidr_block]
  }
}

resource "aws_security_group" "dgraph_alb" {
  name        = "dgraph-alb"
  description = "Security group associated with the dgraph loadbalancer sitting in front of dgraph alpha instances."
  vpc_id      = aws_vpc.dgraph.id

  ingress {
    from_port = 8000
    to_port   = 8000
    protocol  = "tcp"

    security_groups = [aws_security_group.dgraph_client.id]
  }

  ingress {
    from_port = 8080
    to_port   = 8080
    protocol  = "tcp"

    security_groups = [aws_security_group.dgraph_client.id]
  }

  # Egress to the alpha instances port only.
  egress {
    from_port = 8080
    to_port   = 8080
    protocol  = "tcp"

    cidr_blocks = [var.subnet_cidr_block]
  }
}

resource "aws_security_group" "dgraph_services" {
  name        = "dgraph-services"
  description = "Allow all traffic associated with this security group."
  vpc_id      = aws_vpc.dgraph.id

  ingress {
    from_port   = 5080
    to_port     = 5080
    protocol    = "tcp"
    cidr_blocks = [var.subnet_cidr_block]
    description = "For zero internal GRPC communication."
  }

  ingress {
    from_port   = 6080
    to_port     = 6080
    protocol    = "tcp"
    cidr_blocks = [var.cidr_block]
    description = "For zero external GRPC communication."
  }

  ingress {
    from_port   = 7080
    to_port     = 7080
    protocol    = "tcp"
    cidr_blocks = [var.subnet_cidr_block]
    description = "For alpha internal GRPC communication."
  }

  ingress {
    from_port = 8080
    to_port   = 8080
    protocol  = "tcp"

    security_groups = [aws_security_group.dgraph_alb.id]
    description     = "For alpha external HTTP communication."
  }

  ingress {
    from_port   = 9080
    to_port     = 9080
    protocol    = "tcp"
    cidr_blocks = [var.cidr_block]
    description = "For alpha external GRPC communication."
  }

  # Allow egress to everywhere from within any instance in the cluster, this
  # is useful for bootstrap of the instance.
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
