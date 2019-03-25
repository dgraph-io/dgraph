provider "aws" {
  region                  = "${var.region}"
  shared_credentials_file = "${var.shared_credentials_file}"
  profile                 = "${var.profile}"
}

resource "aws_instance" "dgraph_public" {
  ami = "${var.ami}"
  vpc_security_group_ids = ["${aws_security_group.dgraph.id}"]
  instance_type = "${var.instance_type}"
  key_name = "${var.key_pair_name}"

  # This EC2 Instance has a public IP and will be accessible directly from the public Internet
  associate_public_ip_address = true

  tags {
    Name = "${var.instance_name}-public"
  }
}

# ---------------------------------------------------------------------------------------------------------------------
# CREATE A SECURITY GROUP TO CONTROL WHAT REQUESTS CAN GO IN AND OUT OF THE EC2 INSTANCES                              
# ---------------------------------------------------------------------------------------------------------------------
resource "aws_security_group" "dgraph" {
  name = "${var.instance_name}"

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port = "${var.ssh_port}"
    to_port   = "${var.ssh_port}"
    protocol  = "tcp"

    # To keep this setup simple, we allow incoming SSH requests from any IP. In real-world usage, you should only
    # allow SSH requests from trusted servers, such as a bastion host or VPN server.
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port = "${var.dgraph_ui_port}"
    to_port   = "${var.dgraph_ui_port}"
    protocol  = "tcp"

    # To keep this setup simple, we allow incoming SSH requests from any IP. In real-world usage, you should only
    # allow SSH requests from trusted servers, such as a bastion host or VPN server.
    cidr_blocks = ["0.0.0.0/0"]
  }

}

# ---------------------------------------------------------------------------------------------------------------------
# Provision the server using remote-exec
# ---------------------------------------------------------------------------------------------------------------------

resource "null_resource" "dgraph_provisioner" {
  triggers {
    public_ip = "${aws_instance.dgraph_public.public_ip}"
  }

  connection {
    type = "ssh"
    host = "${aws_instance.dgraph_public.public_ip}"
    user = "${var.ssh_user}"
    private_key = "${file("creds/aws.pem")}"
    port = "${var.ssh_port}"
    agent = true
  }

  # copy dgraph service files to the server
  provisioner "file" {
    source      = "files/"
    destination = "/tmp/"
  }

  # change permissions to executable and pipe its output into a new file
  provisioner "remote-exec" {
    inline = [    
      "wget https://github.com/dgraph-io/dgraph/releases/download/v${var.dgraph_version}/dgraph-linux-amd64.tar.gz",
      "sudo tar -C /usr/local/bin -xzf dgraph-linux-amd64.tar.gz",
      "sudo groupadd --system dgraph",
      "sudo useradd --system -d /var/run/dgraph -s /bin/false -g dgraph dgraph",
      "sudo mkdir -p /var/log/dgraph",
      "sudo mkdir -p /var/run/dgraph/",
      "sudo chown -R dgraph:dgraph /var/run/dgraph",
      "sudo chown -R dgraph:dgraph /var/log/dgraph",
      "sudo cp /tmp/dgraph* /etc/systemd/system/",
      "sudo chmod +x /etc/systemd/system/dgraph*",
      "sudo systemctl daemon-reload",
      "sudo systemctl enable --now dgraph",
      "sudo systemctl enable --now dgraph-ui"
    ]
  }
}
