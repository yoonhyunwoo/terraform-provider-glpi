resource "glpi_software" "editor" {
  name = "Editor"
}

resource "glpi_software_license_type" "subscription" {
  name = "Subscription"
}

resource "glpi_software_license" "team" {
  name        = "Editor team plan"
  software_id = glpi_software.editor.id
  type_id     = glpi_software_license_type.subscription.id
  number      = 25
  expire      = "2027-12-31"
}
