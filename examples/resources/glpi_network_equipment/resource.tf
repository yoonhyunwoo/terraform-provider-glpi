resource "glpi_manufacturer" "fortinet" {
  name = "Fortinet"
}

resource "glpi_network_equipment" "firewall" {
  name            = "firewall01"
  serial          = "FG100F0000000001"
  manufacturer_id = glpi_manufacturer.fortinet.id
}
