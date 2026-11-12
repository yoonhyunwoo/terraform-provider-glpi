resource "glpi_computer" "db" {
  name = "db-01"
}

resource "glpi_infocom" "db" {
  item_type = "Computer"
  item_id   = glpi_computer.db.id
  buy_date  = "2026-01-15"
  value     = 4917000
  sink_time = 5
  sink_type = 2
}
