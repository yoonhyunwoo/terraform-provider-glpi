data "glpi_computer_type" "by_name" {
  name = "example"
}

data "glpi_computer_type" "by_id" {
  id = "42"
}
