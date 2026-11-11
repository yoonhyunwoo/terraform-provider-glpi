package provider_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/provider"
)

var protoV6Factories = map[string]func() (tfprotov6.ProviderServer, error){
	"glpi": providerserver.NewProtocol6WithError(provider.New("test")()),
}

// Acceptance tests need a live GLPI: eval "$(testenv/bootstrap.sh)" sets TF_ACC, GLPI_URL and GLPI_USER_TOKEN.
func preCheck(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GLPI_URL", "GLPI_USER_TOKEN"} {
		if os.Getenv(k) == "" {
			t.Fatalf("%s must be set for acceptance tests", k)
		}
	}
}

func computerConfig(name, serial string) string {
	return fmt.Sprintf(`
resource "glpi_manufacturer" "m" {
  name = "tfacc-vendor"
}

resource "glpi_location" "root" {
  name = "tfacc-dc"
}

resource "glpi_location" "rack" {
  name      = "tfacc-rack"
  parent_id = glpi_location.root.id
}

resource "glpi_computer" "c" {
  name            = %q
  serial          = %q
  manufacturer_id = glpi_manufacturer.m.id
  location_id     = glpi_location.rack.id
}

data "glpi_computer" "by_name" {
  name = glpi_computer.c.name
}

data "glpi_location" "by_id" {
  id = glpi_location.rack.id
}
`, name, serial)
}

func TestAccComputer_lifecycle(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: computerConfig("tfacc-host", "SN-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("glpi_computer.c", "serial", "SN-1"),
					resource.TestCheckResourceAttrPair("glpi_computer.c", "manufacturer_id", "glpi_manufacturer.m", "id"),
					resource.TestCheckResourceAttr("glpi_location.rack", "completename", "tfacc-dc > tfacc-rack"),
					resource.TestCheckResourceAttrPair("data.glpi_computer.by_name", "id", "glpi_computer.c", "id"),
					resource.TestCheckResourceAttr("data.glpi_computer.by_name", "serial", "SN-1"),
					resource.TestCheckResourceAttr("data.glpi_location.by_id", "name", "tfacc-rack"),
				),
			},
			{
				Config: computerConfig("tfacc-host-renamed", "SN-2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("glpi_computer.c", "name", "tfacc-host-renamed"),
					resource.TestCheckResourceAttr("glpi_computer.c", "serial", "SN-2"),
				),
			},
			{
				ResourceName:      "glpi_computer.c",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "glpi_location.rack",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccNetworkEquipment_lifecycle(t *testing.T) {
	cfg := func(comment string) string {
		return fmt.Sprintf(`
resource "glpi_network_equipment" "sw" {
  name    = "tfacc-switch"
  comment = %q
}
`, comment)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{Config: cfg("first"), Check: resource.TestCheckResourceAttr("glpi_network_equipment.sw", "comment", "first")},
			{Config: cfg("second"), Check: resource.TestCheckResourceAttr("glpi_network_equipment.sw", "comment", "second")},
			{ResourceName: "glpi_network_equipment.sw", ImportState: true, ImportStateVerify: true},
		},
	})
}
