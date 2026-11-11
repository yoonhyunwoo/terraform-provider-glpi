# Changelog

## 0.1.0 (2026-11-11)

* New resources `glpi_computer`, `glpi_network_equipment`, `glpi_location` and `glpi_manufacturer`.
* New data sources with the same names, looked up by `id` or exact `name`.
* The provider authenticates against the GLPI 10.0 REST API with `user_token` and an optional `app_token`.
* The `purge_on_destroy` provider option deletes items permanently instead of moving them to the GLPI trash bin.
