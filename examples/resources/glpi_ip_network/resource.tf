resource "glpi_ip_network" "office" {
  name    = "Office"
  address = "192.168.50.0"
  netmask = "255.255.254.0"
  gateway = "192.168.50.1"
}
