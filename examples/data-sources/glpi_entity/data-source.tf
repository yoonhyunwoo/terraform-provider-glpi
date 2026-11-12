data "glpi_entity" "by_name" {
  name = "Root entity"
}

# The root entity has ID 0, which only the name lookup can address.
data "glpi_entity" "by_id" {
  id = "2"
}
