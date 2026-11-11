# terraform-provider-glpi

Terraform provider for [GLPI](https://github.com/glpi-project/glpi) assets, built on the GLPI 10.0 legacy REST API (`apirest.php`) and `terraform-plugin-framework`. Documentation for every resource and data source is under [docs/](docs/).

| Resource and data source | GLPI itemtype |
|---|---|
| [`glpi_computer`](docs/resources/computer.md) | `Computer` |
| [`glpi_network_equipment`](docs/resources/network_equipment.md) | `NetworkEquipment` |
| [`glpi_location`](docs/resources/location.md) | `Location` |
| [`glpi_manufacturer`](docs/resources/manufacturer.md) | `Manufacturer` |

Data sources look up items by `id` or exact `name`. Resources support import by numeric ID. Reference fields (`location_id`, `manufacturer_id`, `state_id`, `type_id`, `model_id`, `entity_id`) take GLPI IDs; `0` means unset.

## Behavior

- Destroy moves items to the GLPI trash bin. Set `purge_on_destroy = true` (or `GLPI_PURGE_ON_DESTROY=true`) to delete permanently. Items trashed outside Terraform count as deleted.
- Optional attributes removed from configuration keep their last value in GLPI.
- GLPI 10.0 stores `&`, `<`, and `>` as HTML entities; the provider unescapes them in state.
- Lookup by name inspects at most 1,000 candidate rows. Use `id` when names are not unique.
- Only GLPI 10.0 is supported. GLPI 11 is not supported yet.

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

Acceptance tests run against a disposable GLPI 10.0 in Docker (`testenv/`):

```sh
eval "$(testenv/bootstrap.sh)"   # starts GLPI and MySQL, enables the API, prints env vars
make test testacc
make testenv-down
```

`GLPI_VERSION` sets the image tag (default `10.0`) and `GLPI_PORT` the host port (default `8088`). The bootstrap script generates user tokens through the GLPI CLI because GLPI stores tokens encrypted and rejects raw database values. Run `make generate` after changing a schema, and commit `docs/`; CI fails when it is stale.

## Releasing

Pushing a tag such as `v0.1.0` runs `.github/workflows/release.yml`, which builds, signs, and publishes artifacts with GoReleaser. It requires repository secrets `GPG_PRIVATE_KEY` and `PASSPHRASE`. The Terraform Registry requires an RSA GPG key.
