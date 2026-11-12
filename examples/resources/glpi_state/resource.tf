resource "glpi_state" "in_use" {
  name             = "In use"
  helpdesk_visible = true
}

resource "glpi_state" "repair" {
  name      = "In repair"
  parent_id = glpi_state.in_use.id
}
