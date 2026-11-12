resource "glpi_software" "editor" {
  name = "Editor"
}

resource "glpi_software_version" "v2" {
  name        = "2.0"
  software_id = glpi_software.editor.id
}
