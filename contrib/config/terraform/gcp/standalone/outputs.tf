# ----------------------------------------------------------------------------------
# The ouptut contains the IP address associated with the compute instance. The Ratel
# UI is then accessible using <Instance_IP>:8000
# ----------------------------------------------------------------------------------
output dgraph_ip {
  value = length(google_compute_instance.dgraph_standalone.network_interface.0.access_config) == 0 ? "" : google_compute_instance.dgraph_standalone.network_interface.0.access_config.0.nat_ip
}
