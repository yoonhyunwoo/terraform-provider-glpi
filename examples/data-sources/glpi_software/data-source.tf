data "glpi_software" "by_name" {
  name = "example"
}

data "glpi_software" "by_id" {
  id = "42"
}
