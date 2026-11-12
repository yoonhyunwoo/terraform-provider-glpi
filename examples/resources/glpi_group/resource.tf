resource "glpi_group" "operations" {
  name       = "Operations"
  item_group = true
  user_group = true
}
