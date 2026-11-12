data "glpi_software_category" "by_name" {
  name = "example"
}

data "glpi_software_category" "by_id" {
  id = "42"
}
