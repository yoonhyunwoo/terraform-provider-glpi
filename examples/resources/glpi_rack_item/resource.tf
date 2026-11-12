resource "glpi_rack" "a01" {
  name  = "A01"
  units = 42
}

resource "glpi_computer" "db" {
  name = "db-01"
}

resource "glpi_rack_item" "db" {
  rack_id   = glpi_rack.a01.id
  item_type = "Computer"
  item_id   = glpi_computer.db.id
  position  = 10
}
