resource "glpi_datacenter" "main" {
  name = "Main data center"
}

resource "glpi_datacenter_room" "hall_1" {
  name          = "Hall 1"
  datacenter_id = glpi_datacenter.main.id
  columns       = 9
  rows          = 23
}

resource "glpi_rack" "a01" {
  name               = "A01"
  datacenter_room_id = glpi_datacenter_room.hall_1.id
  units              = 42
  position           = "3,2"
  max_power          = 2200
}
