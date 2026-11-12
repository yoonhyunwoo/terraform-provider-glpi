resource "glpi_network_name" "gateway" {
  name = "gateway"
}

resource "glpi_ip_address" "gateway" {
  name      = "192.168.50.1"
  item_type = "NetworkName"
  item_id   = glpi_network_name.gateway.id
}
