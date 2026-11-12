resource "glpi_datacenter" "main" {
  name = "Main data center"
}

resource "glpi_datacenter_room" "hall_1" {
  name          = "Hall 1"
  datacenter_id = glpi_datacenter.main.id
  columns       = 9
  rows          = 23
}
