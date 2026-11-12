# terraform-provider-glpi

Terraform provider for [GLPI](https://github.com/glpi-project/glpi) assets, built on the GLPI legacy REST API (`apirest.php`) and `terraform-plugin-framework`. It supports GLPI 10.0 and 11.0. Documentation for every resource and data source is under [docs/](docs/).

## Coverage

Every resource has a data source of the same name that looks an item up by `id`, or by exact `name` for items that have one (`rack_item` and `infocom` have none). Prefix each name below with `glpi_`.

| Area | Resources and data sources |
|---|---|
| Assets | `computer`, `network_equipment`, `peripheral` |
| Sites and racks | `location`, `datacenter`, `datacenter_room`, `rack`, `rack_item` |
| Software | `software`, `software_version`, `software_license`, `software_license_type`, `software_category` |
| Networks | `network`, `ip_network`, `network_name`, `ip_address` |
| Dropdowns | `state`, `group`, `manufacturer`, `computer_type`, `rack_type`, `rack_model`, `operating_system`, `operating_system_version` |
| Finance | `infocom` |
| Fields plugin | `plugin_fields_values` (resource), `plugin_fields_container` (data source) |
| Read-only data sources | `user`, `entity`, `device_processor`, `device_memory`, `device_graphic_card` |

Resources support import by numeric ID, except `plugin_fields_values`, which imports as `itemtype/item_id/container_id`. Reference attributes such as `location_id`, `state_id` or `type_id` take GLPI IDs, and `0` means unset. The data sources above turn a name into the ID.

## Behavior

- Destroy moves items that have a GLPI trash bin to it. Set `purge_on_destroy = true` (or `GLPI_PURGE_ON_DESTROY=true`) to delete permanently. Items without a trash bin, such as states, groups and types, are always deleted permanently. Items trashed outside Terraform count as deleted.
- Optional attributes removed from configuration keep their last value in GLPI.
- GLPI 10.0 stores `&`, `<`, and `>` as HTML entities and the provider unescapes them in state. GLPI 11 stores text unchanged, so the provider leaves it alone. The provider reads the server version when it opens a session.
- Lookup by name inspects at most 1,000 candidate rows. Use `id` when names are not unique.
- `item_type` and `item_id` on `rack_item` and `infocom`, and the parent IDs on software versions and licenses, cannot change in place, so changing them replaces the resource.
- GLPI chooses the parent of an `ip_network` from its address. The `parent_id` attribute is read-only.
- `plugin_fields_values` needs GLPI 11.0.2 or later and the Fields plugin 1.23.3 or later. It manages the fields listed in `values` and empties them on destroy. Dropdown, GLPI item and multiple-value fields are not supported.

## Installation

The provider is published on the [Terraform Registry](https://registry.terraform.io/providers/yoonhyunwoo/glpi/latest):

```hcl
terraform {
  required_providers {
    glpi = {
      source  = "yoonhyunwoo/glpi"
      version = "~> 0.1"
    }
  }
}
```

## Configuration

```hcl
provider "glpi" {
  url        = "https://glpi.example.com/apirest.php" # or GLPI_URL
  user_token = var.glpi_user_token                    # or GLPI_USER_TOKEN
  app_token  = var.glpi_app_token                     # optional, or GLPI_APP_TOKEN
  # purge_on_destroy = true                           # permanent delete instead of trash bin
}
```

Enable the API under **Setup > General > API**, add an API client matching your IP, and copy the **Remote access key** from user preferences. Use HTTPS: the provider warns on plain HTTP to non-loopback hosts and refuses redirects.

## Development

Acceptance tests run against a disposable GLPI in Docker (`testenv/`), GLPI 10.0 by default:

```sh
eval "$(testenv/bootstrap.sh)"   # starts GLPI and MySQL, enables the API, prints env vars
make test testacc
make testenv-down
```

`GLPI_VERSION` sets the image tag (default `10.0`) and `GLPI_PORT` the host port (default `8088`). With `GLPI_VERSION=11.0` the script also installs the Fields plugin (`GLPI_FIELDS_VERSION` picks its version), which the Fields tests need; they skip themselves when the plugin is missing. The bootstrap script generates user tokens through the GLPI CLI because GLPI stores tokens encrypted and rejects raw database values. Run `make generate` after changing a schema, and commit `docs/`; CI fails when it is stale.

## Releasing

Pushing a tag such as `v0.1.0` runs `.github/workflows/release.yml`, which builds, signs, and publishes artifacts with GoReleaser. It requires repository secrets `GPG_PRIVATE_KEY` and `PASSPHRASE`. The Terraform Registry requires an RSA GPG key.
