resource "glpi_location" "idc" {
  name = "IDC"
}

resource "glpi_location" "rack_a1" {
  name      = "Rack A1"
  parent_id = glpi_location.idc.id
}
