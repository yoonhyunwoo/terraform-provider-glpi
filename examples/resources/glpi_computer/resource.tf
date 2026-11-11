resource "glpi_manufacturer" "dell" {
  name = "Dell"
}

resource "glpi_location" "idc" {
  name = "IDC"
}

resource "glpi_computer" "build_agent" {
  name            = "build-agent-01"
  serial          = "SN-0001"
  manufacturer_id = glpi_manufacturer.dell.id
  location_id     = glpi_location.idc.id
}
