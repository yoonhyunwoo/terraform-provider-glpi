resource "glpi_manufacturer" "logitech" {
  name = "Logitech"
}

resource "glpi_peripheral" "keyboard" {
  name            = "keyboard-01"
  serial          = "KB-0001"
  manufacturer_id = glpi_manufacturer.logitech.id
}
