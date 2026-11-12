resource "glpi_location" "idc" {
  name = "IDC"
}

resource "glpi_datacenter" "main" {
  name        = "Main data center"
  location_id = glpi_location.idc.id
}
