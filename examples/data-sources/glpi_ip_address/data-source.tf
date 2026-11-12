data "glpi_ip_address" "by_name" {
  name = "192.168.50.1"
}

data "glpi_ip_address" "by_id" {
  id = "42"
}
