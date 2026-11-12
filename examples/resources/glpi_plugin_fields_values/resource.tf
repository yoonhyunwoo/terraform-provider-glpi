data "glpi_plugin_fields_container" "cost" {
  label = "Cost"
}

resource "glpi_computer" "db" {
  name = "db-01"
}

# Field names come from the container's fields attribute.
resource "glpi_plugin_fields_values" "db" {
  item_type    = "Computer"
  item_id      = glpi_computer.db.id
  container_id = data.glpi_plugin_fields_container.cost.id
  values = {
    management_no = "IDC1263224"
    transfer_date = "2026-03-04"
  }
}
