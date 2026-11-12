data "glpi_network_name" "by_name" {
  name = "example"
}

data "glpi_network_name" "by_id" {
  id = "42"
}
