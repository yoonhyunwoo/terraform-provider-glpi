data "glpi_computer" "by_name" {
  name = "example"
}

data "glpi_computer" "by_id" {
  id = "42"
}
