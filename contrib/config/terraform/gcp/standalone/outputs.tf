# ----------------------------------------------------------------------------------
# The output contains the IP address associated with the compute instance.
# Dgraph Alpha is then accessible using <Instance_IP>:8080
# ----------------------------------------------------------------------------------
output dgraph_ip {
  value = length(google_compute_instance.dgraph_standalone.network_interface.0.access_config) == 0 ? "" : google_compute_instance.dgraph_standalone.network_interface.0.access_config.0.nat_ip
}
