data "glpi_software_license" "by_name" {
  name = "example"
}

data "glpi_software_license" "by_id" {
  id = "42"
}
