data "glpi_user" "by_name" {
  name = "jdoe"
}

data "glpi_user" "by_id" {
  id = "42"
}
