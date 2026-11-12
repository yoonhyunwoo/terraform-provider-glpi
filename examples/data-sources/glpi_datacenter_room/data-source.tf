data "glpi_datacenter_room" "by_name" {
  name = "example"
}

data "glpi_datacenter_room" "by_id" {
  id = "42"
}
