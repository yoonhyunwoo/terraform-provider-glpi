data "glpi_device_graphic_card" "by_name" {
  name = "NVIDIA RTX A6000"
}

data "glpi_device_graphic_card" "by_id" {
  id = "42"
}
