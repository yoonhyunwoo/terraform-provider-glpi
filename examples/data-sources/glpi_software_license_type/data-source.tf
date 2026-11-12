data "glpi_software_license_type" "by_name" {
  name = "example"
}

data "glpi_software_license_type" "by_id" {
  id = "42"
}
