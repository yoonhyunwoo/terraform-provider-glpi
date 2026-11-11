data "glpi_network_equipment" "by_name" {
  name = "example"
}

data "glpi_network_equipment" "by_id" {
  id = "42"
}
