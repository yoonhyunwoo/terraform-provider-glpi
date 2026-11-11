# terraform-provider-glpi

Terraform provider for [GLPI](https://github.com/glpi-project/glpi) assets, built on the legacy REST API (`apirest.php`) and `terraform-plugin-framework`.

| Resource and data source | GLPI itemtype |
|---|---|
| `glpi_computer` | Computer |
| `glpi_network_equipment` | NetworkEquipment |
| `glpi_location` | Location |
| `glpi_manufacturer` | Manufacturer |

Data sources look an item up by `id` or by exact `name`. Resources support import by numeric ID. Reference fields (`location_id`, `manufacturer_id`, `state_id`, `type_id`, `model_id`, `entity_id`) take GLPI IDs; `0` means unset.

Behavior to know:

- Destroy moves the item to the GLPI trash bin, where it can be restored. Set `purge_on_destroy = true` (or `GLPI_PURGE_ON_DESTROY=true`) to delete permanently. An item trashed outside Terraform counts as deleted.
- Optional attributes removed from the configuration keep their last value in GLPI.

## Configuration

```hcl
provider "glpi" {
  url        = "https://glpi.example.com/apirest.php" # or GLPI_URL
  user_token = var.glpi_user_token                    # or GLPI_USER_TOKEN
  app_token  = var.glpi_app_token                     # optional, or GLPI_APP_TOKEN
  # purge_on_destroy = true                           # permanent delete instead of trash bin
}
```

Enable the API under Setup > General > API, add an API client that allows your source address, and copy the "Remote access key" from the user's preferences. See [examples/main.tf](examples/main.tf).

## Development

Acceptance tests run against a disposable GLPI 10.0 in Docker (`testenv/`):

```sh
eval "$(testenv/bootstrap.sh)"   # starts GLPI + MySQL, enables the API, prints env vars
make test testacc
make testenv-down
```

`GLPI_VERSION` selects the image tag (default `10.0`); `GLPI_PORT` the host port (default `8088`).
