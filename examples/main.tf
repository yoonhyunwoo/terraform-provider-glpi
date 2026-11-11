terraform {
  required_providers {
    glpi = {
      source = "yoonhyunwoo/glpi"
    }
  }
}

# GLPI_URL and GLPI_USER_TOKEN (and optionally GLPI_APP_TOKEN) can be set instead.
provider "glpi" {
  url        = "https://glpi.example.com/apirest.php"
  user_token = var.glpi_user_token
}

variable "glpi_user_token" {
  type      = string
  sensitive = true
}

resource "glpi_manufacturer" "fortinet" {
  name = "Fortinet"
}

resource "glpi_location" "idc" {
  name = "IDC"
}

resource "glpi_location" "rack_a1" {
  name      = "Rack A1"
  parent_id = glpi_location.idc.id
}

resource "glpi_network_equipment" "firewall" {
  name            = "firewall01"
  serial          = "FG100F0000000001"
  manufacturer_id = glpi_manufacturer.fortinet.id
  location_id     = glpi_location.rack_a1.id
}

data "glpi_computer" "existing" {
  name = "build-agent-01"
}

output "existing_computer_serial" {
  value = data.glpi_computer.existing.serial
}
