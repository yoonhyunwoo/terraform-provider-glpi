resource "glpi_manufacturer" "vendor" {
  name = "Vendor"
}

resource "glpi_software_category" "productivity" {
  name = "Productivity"
}

resource "glpi_software" "editor" {
  name            = "Editor"
  manufacturer_id = glpi_manufacturer.vendor.id
  category_id     = glpi_software_category.productivity.id
}
