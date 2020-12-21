resource "aws_network_acl" "lb_nacl" {
  vpc_id     = aws_vpc.vpc.id
  subnet_ids = aws_subnet.lb_subnets.*.id


  ingress {
    from_port  = 1024
    to_port    = 65535
    rule_no    = 100
    action     = "allow"
    protocol   = "tcp"
    cidr_block = cidrsubnet(var.cidr, 1, 1)
  }

  ingress {
    from_port  = 1024
    to_port    = 65535
    rule_no    = 101
    action     = "allow"
    protocol   = "udp"
    cidr_block = cidrsubnet(var.cidr, 1, 1)
  }

  dynamic "ingress" {
    for_each = var.ingress_ports
    content {
      from_port  = ingress.value
      to_port    = ingress.value
      rule_no    = 102 + ingress.key
      action     = "allow"
      protocol   = "tcp"
      cidr_block = "0.0.0.0/0"
    }
  }

  egress {
    from_port  = 0
    to_port    = 0
    rule_no    = 100
    action     = "allow"
    protocol   = "-1"
    cidr_block = "0.0.0.0/0"
  }

  tags = map(
    "Name", "${var.cluster_name}-lb-nacl",
  )

  depends_on = [aws_eip.nat_eip]
}


resource "aws_network_acl" "nat_nacl" {
  vpc_id     = aws_vpc.vpc.id
  subnet_ids = [aws_subnet.nat_subnet.id]

  ingress {
    from_port  = 0
    to_port    = 65535
    rule_no    = 100
    action     = "allow"
    protocol   = "tcp"
    cidr_block = "0.0.0.0/0"
  }

  ingress {
    from_port  = 0
    to_port    = 65535
    rule_no    = 200
    action     = "allow"
    protocol   = "udp"
    cidr_block = "0.0.0.0/0"
  }

  egress {
    from_port  = 0
    to_port    = 0
    rule_no    = 100
    action     = "allow"
    protocol   = "-1"
    cidr_block = "0.0.0.0/0"
  }

  tags = map(
    "Name", "${var.cluster_name}-nat-nacl",
  )
}


resource "aws_network_acl" "db_nacl" {
  vpc_id     = aws_vpc.vpc.id
  subnet_ids = aws_subnet.db_subnets.*.id

  ingress {
    from_port  = 0
    to_port    = 65535
    rule_no    = 100
    action     = "allow"
    protocol   = "tcp"
    cidr_block = cidrsubnet(var.cidr, 1, 1)
  }

  ingress {
    from_port  = 0
    to_port    = 65535
    rule_no    = 101
    action     = "allow"
    protocol   = "udp"
    cidr_block = cidrsubnet(var.cidr, 1, 1)
  }

  ingress {
    from_port  = 443
    to_port    = 443
    rule_no    = 102
    action     = "allow"
    protocol   = "tcp"
    cidr_block = cidrsubnet(var.cidr, 2, 0)
  }

  ingress {
    from_port  = 1024
    to_port    = 65535
    rule_no    = 103
    action     = "allow"
    protocol   = "tcp"
    cidr_block = "0.0.0.0/0"
  }

  ingress {
    from_port  = 1024
    to_port    = 65535
    rule_no    = 104
    action     = "allow"
    protocol   = "udp"
    cidr_block = "0.0.0.0/0"
  }

  egress {
    from_port  = 0
    to_port    = 0
    rule_no    = 100
    action     = "allow"
    protocol   = "-1"
    cidr_block = "0.0.0.0/0"
  }

  tags = map(
    "Name", "${var.cluster_name}-db-nacl",
  )
}
