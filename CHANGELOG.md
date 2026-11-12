# Changelog

## 0.2.0 (unreleased)

* GLPI 11.0 is supported. The provider reads the server version and unescapes HTML entities only on GLPI 10.0 and older, where GLPI stores `&`, `<` and `>` as entities. On GLPI 11 the earlier behavior altered text that contained sequences such as `&amp;`.
* New resources and data sources: `glpi_peripheral`, `glpi_datacenter`, `glpi_datacenter_room`, `glpi_rack`, `glpi_rack_item`, `glpi_state`, `glpi_group`, `glpi_computer_type`, `glpi_rack_type`, `glpi_rack_model`, `glpi_operating_system`, `glpi_operating_system_version`, `glpi_software`, `glpi_software_version`, `glpi_software_license`, `glpi_software_license_type`, `glpi_software_category`, `glpi_infocom`, `glpi_network`, `glpi_ip_network`, `glpi_network_name` and `glpi_ip_address`.
* New data sources without a resource: `glpi_user`, `glpi_entity`, `glpi_device_processor`, `glpi_device_memory` and `glpi_device_graphic_card`.
* New `glpi_plugin_fields_values` resource and `glpi_plugin_fields_container` data source for the Fields plugin (GLPI 11.0.2 and Fields 1.23.3 or later).
* `glpi_computer` gains `uuid`. `glpi_location` gains `code`, `alias`, `address`, `postcode`, `town`, `state`, `country`, `building`, `room`, `latitude`, `longitude` and `altitude`. `glpi_peripheral` also has `brand`.
* Breaking: `glpi_manufacturer` no longer has `entity_id`. GLPI manufacturers have no entity, so GLPI ignored the value and the attribute always read back as 0.
* Data sources and attributes now cover decimal numbers (`value`, `warranty_value`) and booleans.

## 0.1.0 (2026-11-11)

* New resources `glpi_computer`, `glpi_network_equipment`, `glpi_location` and `glpi_manufacturer`.
* New data sources with the same names, looked up by `id` or exact `name`.
* The provider authenticates against the GLPI 10.0 REST API with `user_token` and an optional `app_token`.
* The `purge_on_destroy` provider option deletes items permanently instead of moving them to the GLPI trash bin.
