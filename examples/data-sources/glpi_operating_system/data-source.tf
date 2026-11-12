data "glpi_operating_system" "by_name" {
  name = "example"
}

data "glpi_operating_system" "by_id" {
  id = "42"
}
