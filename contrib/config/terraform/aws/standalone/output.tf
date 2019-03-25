output dgraph_ip {
    value = "${aws_instance.dgraph_public.public_ip}"
}