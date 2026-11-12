data "glpi_datacenter" "by_name" {
  name = "example"
}

data "glpi_datacenter" "by_id" {
  id = "42"
}
