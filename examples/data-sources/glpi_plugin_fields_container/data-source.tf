data "glpi_plugin_fields_container" "cost" {
  label = "Cost"
}

output "field_names" {
  value = [for f in data.glpi_plugin_fields_container.cost.fields : f.name]
}
