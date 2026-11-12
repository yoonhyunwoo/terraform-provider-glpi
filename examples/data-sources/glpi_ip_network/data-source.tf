data "glpi_ip_network" "by_name" {
  name = "example"
}

data "glpi_ip_network" "by_id" {
  id = "42"
}
