resource "glpi_ip_network" "office" {
  name    = "Office"
  address = "192.168.50.0"
  netmask = "255.255.254.0"
}

resource "glpi_network_name" "printer_vlan" {
  name          = "printer-vlan"
  ip_network_id = glpi_ip_network.office.id
}
