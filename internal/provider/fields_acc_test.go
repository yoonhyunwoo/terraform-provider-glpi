package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/glpi"
)

type fieldsFixture struct {
	containerID int64
	label       string
	computerID  int64
	// names maps the label of each field to the key GLPI generated for it.
	names map[string]string
}

// newFieldsFixture creates a container with one field per supported kind and a computer to hold the
// values. It skips the test when the instance has no usable Fields plugin.
func newFieldsFixture(t *testing.T, c *glpi.Client) fieldsFixture {
	t.Helper()
	ctx := context.Background()
	if _, err := c.List(ctx, fieldsContainerItemtype); err != nil {
		t.Skipf("the Fields plugin is not available on this GLPI: %v", err)
	}
	suffix := acctest.RandString(6)
	fx := fieldsFixture{label: "tfacc " + suffix, names: map[string]string{}}
	var err error
	fx.containerID, err = c.Create(ctx, fieldsContainerItemtype, glpi.Item{
		"label": fx.label, "type": "tab", "itemtypes": []string{"Computer"}, "is_active": 1,
	})
	if err != nil {
		t.Fatalf("creating container: %v", err)
	}
	t.Cleanup(func() {
		fresh := accClient(t)
		if err := fresh.Delete(ctx, "Computer", fx.computerID); err != nil {
			t.Logf("cleanup computer: %v", err)
		}
		if err := fresh.Delete(ctx, fieldsContainerItemtype, fx.containerID); err != nil {
			t.Logf("cleanup container: %v", err)
		}
	})
	for label, typ := range map[string]string{"product": "text", "moved": "date", "price": "number", "active": "yesno", "note": "textarea", "site": "url", "locked": "text"} {
		readOnly := 0
		if label == "locked" {
			readOnly = 1
		}
		if _, err := c.Create(ctx, fieldsFieldItemtype, glpi.Item{
			"label": label, "type": typ, "plugin_fields_containers_id": fx.containerID, "is_readonly": readOnly,
		}); err != nil {
			t.Fatalf("creating field %s: %v", label, err)
		}
	}
	fields, err := c.List(ctx, fieldsFieldItemtype)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fields {
		if cid, _ := strconv.ParseInt(rowString(f["plugin_fields_containers_id"]), 10, 64); cid == fx.containerID {
			fx.names[rowString(f["label"])] = rowString(f["name"])
		}
	}
	fx.computerID, err = c.Create(ctx, "Computer", glpi.Item{"name": "tfacc-" + suffix + "-fields"})
	if err != nil {
		t.Fatalf("creating computer: %v", err)
	}
	return fx
}

func (fx fieldsFixture) config(values string) string {
	return fmt.Sprintf(`
data "glpi_plugin_fields_container" "c" {
  label = %[1]q
}

resource "glpi_plugin_fields_values" "v" {
  item_type    = "Computer"
  item_id      = %[2]d
  container_id = data.glpi_plugin_fields_container.c.id
  values = {
%[3]s
  }
}
`, fx.label, fx.computerID, values)
}

func (fx fieldsFixture) storedValue(c *glpi.Client, label string) (string, error) {
	ctx := context.Background()
	row, err := c.Get(ctx, fieldsContainerItemtype, fx.containerID)
	if err != nil {
		return "", err
	}
	ct, err := loadFieldsContainer(ctx, c, row)
	if err != nil {
		return "", err
	}
	rows, err := c.SubItems(ctx, "Computer", fx.computerID, ct.valuesItemtype("Computer"))
	if err != nil || len(rows) == 0 {
		return "", err
	}
	return rowString(rows[0][fx.names[label]]), nil
}

func TestAccFieldsValuesRoundTrip(t *testing.T) {
	c := accClient(t)
	fx := newFieldsFixture(t, c)
	n := fx.names
	addr := "glpi_plugin_fields_values.v"
	first := fmt.Sprintf(`    %[1]s = "srv & <x> &amp;"
    %[2]s = "2026-03-04"
    %[3]s = "12.5"
    %[4]s = "1"
    %[5]s = "line one"
    %[6]s = "https://example.com/a?b=c"`, n["product"], n["moved"], n["price"], n["active"], n["note"], n["site"])
	second := fmt.Sprintf(`    %[1]s = "renamed"
    %[2]s = "2026-04-05"
    %[3]s = "12.5"
    %[4]s = "0"`, n["product"], n["moved"], n["price"], n["active"])
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: specFactories,
		CheckDestroy: func(*terraform.State) error {
			for label, want := range map[string]string{"product": "", "active": "0"} {
				got, err := fx.storedValue(c, label)
				if err != nil {
					return err
				}
				if got != want {
					return fmt.Errorf("field %s still holds %q after destroy", label, got)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fx.config(first),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", fmt.Sprintf("Computer/%d/%d", fx.computerID, fx.containerID)),
					resource.TestCheckResourceAttr(addr, "values."+n["product"], "srv & <x> &amp;"),
					resource.TestCheckResourceAttr(addr, "values."+n["moved"], "2026-03-04"),
					resource.TestCheckResourceAttr(addr, "values."+n["active"], "1"),
					resource.TestCheckResourceAttr("data.glpi_plugin_fields_container.c", "fields.#", "7"),
					resource.TestCheckResourceAttr("data.glpi_plugin_fields_container.c", "type", "tab"),
				),
			},
			{ResourceName: addr, ImportState: true, ImportStateVerify: true, Config: fx.config(first)},
			{
				Config: fx.config(second),
				Check: func(*terraform.State) error {
					for label, want := range map[string]string{"product": "renamed", "note": "", "site": "", "active": "0"} {
						got, err := fx.storedValue(c, label)
						if err != nil {
							return err
						}
						if got != want {
							return fmt.Errorf("field %s holds %q, want %q (fields dropped from values must be emptied)", label, got, want)
						}
					}
					return nil
				},
			},
		},
	})
}

func TestAccFieldsValuesRejectsFieldsGLPIWouldDrop(t *testing.T) {
	c := accClient(t)
	fx := newFieldsFixture(t, c)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: specFactories,
		Steps: []resource.TestStep{
			{Config: fx.config(`    nosuchfield = "x"`), ExpectError: regexp.MustCompile(`has no field "nosuchfield"`)},
			{Config: fx.config(fmt.Sprintf(`    %s = "x"`, fx.names["locked"])), ExpectError: regexp.MustCompile(`is read-only in GLPI`)},
			{
				Config: fx.config(fmt.Sprintf(`    %s = "x"`, fx.names["product"])) + `resource "glpi_plugin_fields_values" "wrong" {
  item_type    = "Computer"
  item_id      = 1
  container_id = 99999
  values       = {}
}
`,
				ExpectError: regexp.MustCompile(`(?s)Writing Fields values|Not Found|ITEM_NOT_FOUND`),
			},
		},
	})
}
