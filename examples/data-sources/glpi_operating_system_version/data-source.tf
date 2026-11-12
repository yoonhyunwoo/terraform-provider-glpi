data "glpi_operating_system_version" "by_name" {
  name = "example"
}

data "glpi_operating_system_version" "by_id" {
  id = "42"
}
