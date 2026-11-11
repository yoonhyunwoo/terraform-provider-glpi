package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/glpi"
	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/provider"
)

var protoV6Factories = map[string]func() (tfprotov6.ProviderServer, error){
	"glpi": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func preCheck(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GLPI_URL", "GLPI_USER_TOKEN"} {
		if os.Getenv(k) == "" {
			t.Fatalf(`%s must be set for acceptance tests: run eval "$(testenv/bootstrap.sh)"`, k)
		}
	}
}

func testClient() *glpi.Client {
	return glpi.New(glpi.Config{BaseURL: os.Getenv("GLPI_URL"), UserToken: os.Getenv("GLPI_USER_TOKEN")})
}

var itemtypes = map[string]string{
	"glpi_computer":          "Computer",
	"glpi_network_equipment": "NetworkEquipment",
	"glpi_location":          "Location",
	"glpi_manufacturer":      "Manufacturer",
}

func isGone(item glpi.Item) bool {
	n, ok := item["is_deleted"].(json.Number)
	if !ok {
		return false
	}
	i, _ := n.Int64()
	return i != 0
}

func checkDestroy(s *terraform.State) error {
	for key, rs := range s.RootModule().Resources {
		itemtype, ok := itemtypes[rs.Type]
		if !ok || strings.HasPrefix(key, "data.") {
			continue
		}
		id, _ := strconv.ParseInt(rs.Primary.ID, 10, 64)
		item, err := testClient().Get(context.Background(), itemtype, id)
		switch {
		case glpi.IsNotFound(err):
		case err != nil:
			return err
		case !isGone(item):
			return fmt.Errorf("%s %d still exists after destroy", itemtype, id)
		}
	}
	return nil
}

func computerConfig(prefix, name, serial string) string {
	return fmt.Sprintf(`
resource "glpi_manufacturer" "m" {
  name = "%[1]s-vendor"
}

resource "glpi_location" "root" {
  name = "%[1]s-dc"
}

resource "glpi_location" "rack" {
  name      = "%[2]s-rack"
  parent_id = glpi_location.root.id
}

resource "glpi_computer" "c" {
  name            = %[2]q
  serial          = %[3]q
  manufacturer_id = glpi_manufacturer.m.id
  location_id     = glpi_location.rack.id
}

data "glpi_computer" "by_name" {
  name = glpi_computer.c.name
}

data "glpi_location" "by_id" {
  id = glpi_location.rack.id
}

data "glpi_manufacturer" "by_name" {
  name = glpi_manufacturer.m.name
}
`, prefix, name, serial)
}

func TestAccComputer_lifecycle(t *testing.T) {
	prefix := acctest.RandomWithPrefix("tfacc")
	first, renamed := prefix+"-host", prefix+"-renamed"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: computerConfig(prefix, first, "SN-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("glpi_computer.c", "serial", "SN-1"),
					resource.TestCheckResourceAttrPair("glpi_computer.c", "manufacturer_id", "glpi_manufacturer.m", "id"),
					resource.TestCheckResourceAttr("glpi_location.rack", "completename", prefix+"-dc > "+first+"-rack"),
					resource.TestCheckResourceAttrPair("data.glpi_computer.by_name", "id", "glpi_computer.c", "id"),
					resource.TestCheckResourceAttr("data.glpi_computer.by_name", "serial", "SN-1"),
					resource.TestCheckResourceAttr("data.glpi_location.by_id", "name", first+"-rack"),
					resource.TestCheckResourceAttrPair("data.glpi_manufacturer.by_name", "id", "glpi_manufacturer.m", "id"),
				),
			},
			{
				Config: computerConfig(prefix, renamed, "SN-2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("glpi_computer.c", "name", renamed),
					resource.TestCheckResourceAttr("glpi_computer.c", "serial", "SN-2"),
					resource.TestCheckResourceAttr("glpi_location.rack", "completename", prefix+"-dc > "+renamed+"-rack"),
				),
			},
			{ResourceName: "glpi_computer.c", ImportState: true, ImportStateVerify: true},
			{ResourceName: "glpi_location.rack", ImportState: true, ImportStateVerify: true},
			{ResourceName: "glpi_manufacturer.m", ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestAccNetworkEquipment_lifecycle(t *testing.T) {
	name := acctest.RandomWithPrefix("tfacc") + "-switch"
	cfg := func(comment string) string {
		return fmt.Sprintf(`
resource "glpi_network_equipment" "sw" {
  name    = %q
  comment = %q
}

data "glpi_network_equipment" "by_name" {
  name = glpi_network_equipment.sw.name
}
`, name, comment)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{Config: cfg("first"), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("glpi_network_equipment.sw", "comment", "first"),
				resource.TestCheckResourceAttr("data.glpi_network_equipment.by_name", "comment", "first"),
			)},
			{Config: cfg("second"), Check: resource.TestCheckResourceAttr("glpi_network_equipment.sw", "comment", "second")},
			{ResourceName: "glpi_network_equipment.sw", ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestAccManufacturer_specialCharacters(t *testing.T) {
	name := acctest.RandomWithPrefix("tfacc") + " & <co>"
	cfg := fmt.Sprintf(`
resource "glpi_manufacturer" "m" {
  name    = %q
  comment = "R&D <team> \"quoted\" 'single'"
}

data "glpi_manufacturer" "by_name" {
  name = glpi_manufacturer.m.name
}
`, name)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{Config: cfg, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("glpi_manufacturer.m", "name", name),
				resource.TestCheckResourceAttr("glpi_manufacturer.m", "comment", `R&D <team> "quoted" 'single'`),
				resource.TestCheckResourceAttrPair("data.glpi_manufacturer.by_name", "id", "glpi_manufacturer.m", "id"),
			)},
			{Config: cfg, PlanOnly: true},
		},
	})
}

func TestAccManufacturer_deletedOutsideTerraformIsRecreated(t *testing.T) {
	name := acctest.RandomWithPrefix("tfacc") + "-gone"
	cfg := fmt.Sprintf(`resource "glpi_manufacturer" "m" { name = %q }`, name)
	var id string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{Config: cfg, Check: func(s *terraform.State) error {
				id = s.RootModule().Resources["glpi_manufacturer.m"].Primary.ID
				return nil
			}},
			{
				PreConfig: func() {
					n, _ := strconv.ParseInt(id, 10, 64)
					if err := testClient().Delete(context.Background(), "Manufacturer", n); err != nil {
						t.Fatal(err)
					}
				},
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccDataSource_lookupErrors(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config:      `data "glpi_manufacturer" "x" { name = "tfacc-does-not-exist" }`,
				ExpectError: regexp.MustCompile(`No Manufacturer named`),
			},
			{
				Config:      `data "glpi_manufacturer" "x" {}`,
				ExpectError: regexp.MustCompile(`exactly one of id or name`),
			},
			{
				Config:      `data "glpi_manufacturer" "x" { id = "007" }`,
				ExpectError: regexp.MustCompile(`not a positive integer`),
			},
		},
	})
}

func TestAccImport_invalidID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config:        `resource "glpi_manufacturer" "m" { name = "tfacc-import-invalid" }`,
				ResourceName:  "glpi_manufacturer.m",
				ImportState:   true,
				ImportStateId: "abc",
				ExpectError:   regexp.MustCompile(`not a positive integer`),
			},
		},
	})
}

func TestProviderConfigurationErrors(t *testing.T) {
	cfg := `data "glpi_manufacturer" "x" { id = "1" }`
	cases := []struct {
		name string
		env  map[string]string
		err  string
	}{
		{"missing url", map[string]string{"GLPI_URL": "", "GLPI_USER_TOKEN": "t"}, "Missing GLPI URL"},
		{"schemeless url", map[string]string{"GLPI_URL": "glpi.example.com/apirest.php", "GLPI_USER_TOKEN": "t"}, "Invalid GLPI URL"},
		{"missing token", map[string]string{"GLPI_URL": "https://glpi.example.com/apirest.php", "GLPI_USER_TOKEN": ""}, "Missing user token"},
		{"bad purge flag", map[string]string{"GLPI_URL": "https://glpi.example.com/apirest.php", "GLPI_USER_TOKEN": "t", "GLPI_PURGE_ON_DESTROY": "maybe"}, "Invalid GLPI_PURGE_ON_DESTROY"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("GLPI_PURGE_ON_DESTROY", "")
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6Factories,
				Steps:                    []resource.TestStep{{Config: cfg, ExpectError: regexp.MustCompile(c.err)}},
			})
		})
	}
}
