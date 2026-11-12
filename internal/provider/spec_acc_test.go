package provider

import (
	"context"
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
)

var specFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"glpi": providerserver.NewProtocol6WithError(New("test")()),
}

func accClient(t *testing.T) *glpi.Client {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests need TF_ACC and a GLPI: run eval \"$(testenv/bootstrap.sh)\"")
	}
	for _, k := range []string{"GLPI_URL", "GLPI_USER_TOKEN"} {
		if os.Getenv(k) == "" {
			t.Fatalf(`%s must be set for acceptance tests: run eval "$(testenv/bootstrap.sh)"`, k)
		}
	}
	return glpi.New(glpi.Config{BaseURL: os.Getenv("GLPI_URL"), UserToken: os.Getenv("GLPI_USER_TOKEN"), PurgeOnDestroy: true})
}

// anchorOrder lists the itemtypes that other items reference, parents first. Each entry names the
// extra input GLPI needs besides a name, as itemtype:column pairs resolved against earlier anchors.
var anchorOrder = []struct {
	itemtype string
	input    map[string]any
	parents  map[string]string
}{
	{itemtype: "Entity"},
	{itemtype: "Location"},
	{itemtype: "Manufacturer"},
	{itemtype: "State"},
	{itemtype: "ComputerType"},
	{itemtype: "ComputerModel"},
	{itemtype: "NetworkEquipmentType"},
	{itemtype: "NetworkEquipmentModel"},
	{itemtype: "PeripheralType"},
	{itemtype: "PeripheralModel"},
	{itemtype: "Datacenter"},
	{itemtype: "DCRoom", input: map[string]any{"vis_cols": 9, "vis_rows": 9}, parents: map[string]string{"datacenters_id": "Datacenter"}},
	{itemtype: "RackType"},
	{itemtype: "RackModel"},
	{itemtype: "Rack", input: map[string]any{"number_units": 42}},
	{itemtype: "Group"},
	{itemtype: "SoftwareCategory"},
	{itemtype: "SoftwareLicenseType"},
	{itemtype: "OperatingSystem"},
	{itemtype: "Software"},
	{itemtype: "SoftwareVersion", parents: map[string]string{"softwares_id": "Software"}},
	{itemtype: "IPNetwork", input: map[string]any{"network": "10.250.0.0/16"}},
	{itemtype: "NetworkName", input: map[string]any{"itemtype": "", "items_id": 0}},
	{itemtype: "Computer"},
}

type anchors map[string]int64

func createAnchors(t *testing.T, c *glpi.Client, suffix string) anchors {
	t.Helper()
	ctx := context.Background()
	ids := anchors{}
	var created []struct {
		itemtype string
		id       int64
	}
	t.Cleanup(func() {
		// A session opened before the Entity anchor existed may not delete it, so clean up with a new one.
		fresh := accClient(t)
		for i := len(created) - 1; i >= 0; i-- {
			if err := fresh.Delete(ctx, created[i].itemtype, created[i].id); err != nil {
				t.Logf("cleanup %s %d: %v", created[i].itemtype, created[i].id, err)
			}
		}
	})
	for _, a := range anchorOrder {
		input := glpi.Item{"name": "tfacc-" + suffix + "-anchor-" + strings.ToLower(a.itemtype)}
		for k, v := range a.input {
			input[k] = v
		}
		for col, parent := range a.parents {
			input[col] = ids[parent]
		}
		id, err := c.Create(ctx, a.itemtype, input)
		if err != nil {
			t.Fatalf("creating %s anchor: %v", a.itemtype, err)
		}
		ids[a.itemtype] = id
		created = append(created, struct {
			itemtype string
			id       int64
		}{a.itemtype, id})
	}
	return ids
}

// sampleOverrides fixes values GLPI validates beyond their type, keyed by "<itemtype>.<tf attribute>".
var sampleOverrides = map[string]string{
	"Infocom.buy_date":              `"2026-01-15"`,
	"Infocom.use_date":              `"2026-02-01"`,
	"SoftwareLicense.expire":        `"2027-01-15"`,
	"Rack.background_color":         `"#aabbcc"`,
	"Item_Rack.background_color":    `"#112233"`,
	"Rack.position":                 `"3,2"`,
	"Item_Rack.item_type":           `"Computer"`,
	"Infocom.item_type":             `"Computer"`,
	"NetworkName.item_type":         `"Computer"`,
	"IPAddress.item_type":           `"NetworkName"`,
	"IPAddress.name":                `"10.250.1.5"`,
	"IPNetwork.address":             `"10.99.0.0"`,
	"IPNetwork.netmask":             `"255.255.255.0"`,
	"IPNetwork.gateway":             `"10.99.0.1"`,
	"Computer.uuid":                 `"11111111-2222-3333-4444-555555555555"`,
	"Item_Rack.orientation":         "1",
	"Item_Rack.position":            "10",
	"Item_Rack.horizontal_position": "0",
	"Infocom.sink_type":             "2",
	"SoftwareLicense.number":        "5",
	"Rack.units":                    "42",
	"Rack.room_orientation":         "1",
}

// sample returns the HCL literal for a field and whether the value is a string, so the update step
// can tweak it.
func (a anchors) sample(s itemSpec, f field, suffix string, own map[string]int64) (string, bool) {
	if v, ok := sampleOverrides[s.itemtype+"."+f.tf]; ok {
		return v, strings.HasPrefix(v, `"`)
	}
	switch f.kind {
	case kindInt:
		if f.target != "" {
			if id, ok := own[f.tf]; ok {
				return strconv.FormatInt(id, 10), false
			}
			return strconv.FormatInt(a[f.target], 10), false
		}
		return "7", false
	case kindBool:
		return "true", false
	case kindFloat:
		return "1234.5", false
	}
	return strconv.Quote("tfacc-" + suffix + "-" + f.tf), true
}

type specTest struct {
	spec    itemSpec
	suffix  string
	anchors anchors
	own     map[string]int64
	major   int
}

func (st specTest) writable() []field {
	var out []field
	for _, f := range st.spec.fields {
		if !f.readOnly && f.minGLPI <= st.major {
			out = append(out, f)
		}
	}
	return out
}

func (st specTest) resourceConfig(tweak bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "resource %q \"t\" {\n", "glpi_"+st.spec.tfName)
	for _, f := range st.writable() {
		v, isString := st.anchors.sample(st.spec, f, st.suffix, st.own)
		if tweak && st.tweaked(f) && isString {
			v = strings.TrimSuffix(v, `"`) + `-b"`
		}
		if tweak && st.tweaked(f) && !isString && f.kind == kindInt && f.target == "" {
			v = "8"
		}
		fmt.Fprintf(&b, "  %s = %s\n", f.tf, v)
	}
	b.WriteString("}\n")
	return b.String()
}

// tweaked picks the one mutable attribute the update step changes.
func (st specTest) tweaked(f field) bool {
	return f.tf == st.updateField().tf
}

func (st specTest) updateField() field {
	for _, want := range []kind{kindString, kindInt} {
		for _, f := range st.writable() {
			if f.kind == want && !f.immutable && f.target == "" && sampleOverrides[st.spec.itemtype+"."+f.tf] == "" {
				return f
			}
		}
	}
	return field{}
}

func (st specTest) checks(tweak bool) []resource.TestCheckFunc {
	addr := "glpi_" + st.spec.tfName + ".t"
	checks := []resource.TestCheckFunc{resource.TestCheckResourceAttrSet(addr, "id")}
	for _, f := range st.writable() {
		v, isString := st.anchors.sample(st.spec, f, st.suffix, st.own)
		if tweak && st.tweaked(f) {
			if isString {
				v = strings.TrimSuffix(v, `"`) + `-b"`
			} else if f.kind == kindInt && f.target == "" {
				v = "8"
			}
		}
		checks = append(checks, resource.TestCheckResourceAttr(addr, f.tf, strings.Trim(v, `"`)))
	}
	return checks
}

func (st specTest) dataSourceConfig() string {
	var b strings.Builder
	fmt.Fprintf(&b, "data %q \"by_id\" {\n  id = %s.t.id\n}\n", "glpi_"+st.spec.tfName, "glpi_"+st.spec.tfName)
	if _, ok := st.spec.nameField(); ok {
		fmt.Fprintf(&b, "data %q \"by_name\" {\n  name = %s.t.name\n}\n", "glpi_"+st.spec.tfName, "glpi_"+st.spec.tfName)
	}
	return b.String()
}

func (st specTest) dataSourceChecks() []resource.TestCheckFunc {
	addr := "glpi_" + st.spec.tfName + ".t"
	var checks []resource.TestCheckFunc
	lookups := []string{"data.glpi_" + st.spec.tfName + ".by_id"}
	if _, ok := st.spec.nameField(); ok {
		lookups = append(lookups, "data.glpi_"+st.spec.tfName+".by_name")
	}
	for _, ds := range lookups {
		checks = append(checks, resource.TestCheckResourceAttrPair(ds, "id", addr, "id"))
		for _, f := range st.spec.fields {
			checks = append(checks, resource.TestCheckResourceAttrPair(ds, f.tf, addr, f.tf))
		}
	}
	return checks
}

func destroyCheck(c *glpi.Client, itemtype string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if !strings.HasPrefix(rs.Type, "glpi_") {
				continue
			}
			id, _ := strconv.ParseInt(rs.Primary.ID, 10, 64)
			item, err := c.Get(context.Background(), itemtype, id)
			switch {
			case glpi.IsNotFound(err):
			case err != nil:
				return err
			case !isTrashed(item):
				return fmt.Errorf("%s %d still exists after destroy", itemtype, id)
			}
		}
		return nil
	}
}

// TestAccEverySpecRoundTrips creates every managed itemtype with every writable attribute set, reads
// the values back through GLPI, imports the item, changes one attribute in place, looks the item up
// through the data source and destroys it. A column name or type the spec gets wrong shows up as a
// value that does not survive the round trip.
func TestAccEverySpecRoundTrips(t *testing.T) {
	c := accClient(t)
	suffix := acctest.RandString(6)
	ids := createAnchors(t, c, suffix)
	major, err := c.ServerMajor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range itemSpecs {
		if spec.dataOnly {
			continue
		}
		t.Run(spec.tfName, func(t *testing.T) {
			st := specTest{spec: spec, suffix: suffix, anchors: ids, own: map[string]int64{}, major: major}
			st.own = freshParents(t, c, spec, ids, suffix)
			addr := "glpi_" + spec.tfName + ".t"
			base := st.resourceConfig(false)
			steps := []resource.TestStep{
				{
					Config: base + st.dataSourceConfig(),
					Check:  resource.ComposeAggregateTestCheckFunc(append(st.checks(false), st.dataSourceChecks()...)...),
				},
				{ResourceName: addr, ImportState: true, ImportStateVerify: true, Config: base},
			}
			if st.updateField().tf != "" {
				steps = append(steps, resource.TestStep{
					Config: st.resourceConfig(true),
					Check:  resource.ComposeAggregateTestCheckFunc(st.checks(true)...),
				})
			}
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: specFactories,
				CheckDestroy:             destroyCheck(c, spec.itemtype),
				Steps:                    steps,
			})
		})
	}
}

// freshParents creates the per-test parents of items GLPI allows only once per parent.
func freshParents(t *testing.T, c *glpi.Client, spec itemSpec, ids anchors, suffix string) map[string]int64 {
	t.Helper()
	own := map[string]int64{}
	var fresh string
	switch spec.itemtype {
	case "Infocom", "Item_Rack":
		fresh = "Computer"
	default:
		return own
	}
	id, err := c.Create(context.Background(), fresh, glpi.Item{"name": "tfacc-" + suffix + "-" + strings.ToLower(spec.itemtype) + "-item"})
	if err != nil {
		t.Fatalf("creating %s for %s: %v", fresh, spec.itemtype, err)
	}
	t.Cleanup(func() { _ = c.Delete(context.Background(), fresh, id) })
	own["item_id"] = id
	return own
}

// TestAccDataOnlySpecsLookUpItems reads the catalogs and directories the provider exposes as data
// sources only.
func TestAccDataOnlySpecsLookUpItems(t *testing.T) {
	c := accClient(t)
	suffix := acctest.RandString(6)
	for _, spec := range itemSpecs {
		if !spec.dataOnly {
			continue
		}
		t.Run(spec.tfName, func(t *testing.T) {
			name, id, byID := dataOnlyTarget(t, c, spec, suffix)
			addr := "data.glpi_" + spec.tfName
			cfg := fmt.Sprintf("data %q \"by_name\" {\n  name = %q\n}\n", "glpi_"+spec.tfName, name)
			checks := []resource.TestCheckFunc{
				resource.TestCheckResourceAttr(addr+".by_name", "name", name),
				resource.TestCheckResourceAttrSet(addr+".by_name", "id"),
			}
			if byID {
				cfg += fmt.Sprintf("data %q \"by_id\" {\n  id = %q\n}\n", "glpi_"+spec.tfName, strconv.FormatInt(id, 10))
				checks = append(checks, resource.TestCheckResourceAttr(addr+".by_id", "name", name))
			}
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: specFactories,
				Steps:                    []resource.TestStep{{Config: cfg, Check: resource.ComposeAggregateTestCheckFunc(checks...)}},
			})
		})
	}
}

// dataOnlyTarget returns the name of an item the data source can find, creating a catalog entry
// when the instance has none. byID reports whether the item can also be looked up by ID.
func dataOnlyTarget(t *testing.T, c *glpi.Client, spec itemSpec, suffix string) (name string, id int64, byID bool) {
	t.Helper()
	ctx := context.Background()
	switch spec.itemtype {
	case "User":
		return "glpi", 0, false
	case "Entity":
		return "Root entity", 0, true
	}
	nameCol, _ := spec.nameField()
	name = "tfacc-" + suffix + "-" + strings.ToLower(spec.itemtype)
	id, err := c.Create(ctx, spec.itemtype, glpi.Item{nameCol.api: name})
	if err != nil {
		t.Fatalf("creating %s: %v", spec.itemtype, err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, spec.itemtype, id) })
	return name, id, true
}

// TestAccNewerColumnsAreRejectedOnOlderGLPI checks that an attribute GLPI 10 has no column for fails
// at apply time instead of reading back as an empty value.
func TestAccNewerColumnsAreRejectedOnOlderGLPI(t *testing.T) {
	c := accClient(t)
	major, err := c.ServerMajor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if major >= 11 {
		t.Skip("every column exists on this GLPI")
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: specFactories,
		Steps: []resource.TestStep{{
			Config: `
resource "glpi_location" "l" {
  name = "tfacc-newer"
  code = "x"
}
`,
			ExpectError: regexp.MustCompile(`needs GLPI 11 or later`),
		}},
	})
}
