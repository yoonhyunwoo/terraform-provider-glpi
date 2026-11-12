package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/glpi"
)

func TestStateValueNormalisesGLPIQuirks(t *testing.T) {
	str := field{api: "serial", kind: kindString}
	num := field{api: "locations_id", kind: kindInt}
	flg := field{api: "is_valid", kind: kindBool}
	dcm := field{api: "value", kind: kindFloat}
	cases := []struct {
		name string
		f    field
		in   any
		want attr.Value
	}{
		{"null string becomes empty", str, nil, types.StringValue("")},
		{"string passes through", str, "SN1", types.StringValue("SN1")},
		{"number in string field", str, json.Number("7"), types.StringValue("7")},
		{"text is never altered", str, "R&#38;D <team>", types.StringValue("R&#38;D <team>")},
		{"null bool is false", flg, nil, types.BoolValue(false)},
		{"json number bool", flg, json.Number("1"), types.BoolValue(true)},
		{"quoted zero bool", flg, "0", types.BoolValue(false)},
		{"null decimal is zero", dcm, nil, types.Float64Value(0)},
		{"quoted decimal", dcm, "49170000.0000", types.Float64Value(49170000)},
		{"json number decimal", dcm, json.Number("12.5"), types.Float64Value(12.5)},
		{"null int becomes zero", num, nil, types.Int64Value(0)},
		{"empty-string int becomes zero", num, "", types.Int64Value(0)},
		{"quoted int", num, "12", types.Int64Value(12)},
		{"json number int", num, json.Number("12"), types.Int64Value(12)},
	}
	for _, c := range cases {
		got, err := c.f.stateValue(c.in)
		if err != nil || !got.Equal(c.want) {
			t.Errorf("%s: got %v, %v want %v", c.name, got, err, c.want)
		}
	}
	if _, err := num.stateValue(true); err == nil {
		t.Error("an unexpected type must be an error, not a silent zero")
	}
	if _, err := num.stateValue("abc"); err == nil {
		t.Error("a non-numeric int must be an error")
	}
}

func TestIsTrashed(t *testing.T) {
	for in, want := range map[string]bool{
		`{"is_deleted":1}`:    true,
		`{"is_deleted":0}`:    false,
		`{"is_deleted":"1"}`:  true,
		`{"is_deleted":"0"}`:  false,
		`{"is_deleted":true}`: true,
		`{}`:                  false,
	} {
		var item glpi.Item
		dec := json.NewDecoder(strings.NewReader(in))
		dec.UseNumber()
		if err := dec.Decode(&item); err != nil {
			t.Fatal(err)
		}
		if got := isTrashed(item); got != want {
			t.Errorf("%s: got %v want %v", in, got, want)
		}
	}
}

func TestParseIDOnlyAcceptsCanonicalPositive(t *testing.T) {
	for _, ok := range []string{"1", "42", "9007199254740993"} {
		if _, err := parseID(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "0", "-1", "007", "+7", "1.5", "abc", " 7"} {
		if _, err := parseID(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestValidateURL(t *testing.T) {
	for _, ok := range []string{"https://glpi.example.com/apirest.php", "http://127.0.0.1:8088/apirest.php"} {
		if err := validateURL(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"glpi.example.com/apirest.php", "ftp://glpi/apirest.php", "https:///apirest.php", "://x", "https://user:pass@glpi.example.com/apirest.php"} {
		err := validateURL(bad)
		if err == nil {
			t.Errorf("%q accepted", bad)
		} else if strings.Contains(err.Error(), "user:pass") {
			t.Errorf("error echoes the credentials in the rejected URL: %v", err)
		}
	}
}

func TestOrEnvTreatsEmptyAsUnset(t *testing.T) {
	t.Setenv("GLPI_TEST_VAR", "fromenv")
	for _, v := range []types.String{types.StringNull(), types.StringValue("")} {
		if got := orEnv(v, "GLPI_TEST_VAR"); got != "fromenv" {
			t.Errorf("%v: got %q", v, got)
		}
	}
	if got := orEnv(types.StringValue("cfg"), "GLPI_TEST_VAR"); got != "cfg" {
		t.Errorf("configured value lost: %q", got)
	}
}

func TestAddCIDRDerivesThePrefixLengthGLPIRequires(t *testing.T) {
	cases := []struct {
		name, address, netmask, want string
	}{
		{"v4 /23", "192.168.50.0", "255.255.254.0", "192.168.50.0/23"},
		{"v4 /8", "10.0.0.0", "255.0.0.0", "10.0.0.0/8"},
		{"v6 /64", "2001:db8::", "ffff:ffff:ffff:ffff::", "2001:db8::/64"},
		{"missing netmask", "10.0.0.0", "", ""},
		{"unparsable netmask", "10.0.0.0", "abc", ""},
		{"non-contiguous netmask", "10.0.0.0", "255.0.255.0", ""},
		{"missing address", "", "255.0.0.0", ""},
	}
	for _, c := range cases {
		in := glpi.Item{"address": c.address, "netmask": c.netmask}
		addCIDR(in)
		got, _ := in["network"].(string)
		if got != c.want {
			t.Errorf("%s: network %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDetachedByDefaultKeepsOwnersTheCallerSet(t *testing.T) {
	in := glpi.Item{"name": "n"}
	detachedByDefault(in)
	if in["itemtype"] != "" || in["items_id"] != 0 {
		t.Errorf("owner columns not defaulted: %v", in)
	}
	owned := glpi.Item{"itemtype": "Computer", "items_id": int64(5)}
	detachedByDefault(owned)
	if owned["itemtype"] != "Computer" || owned["items_id"] != int64(5) {
		t.Errorf("explicit owner overwritten: %v", owned)
	}
}

func TestCheckVersionRejectsOnlyConfiguredAttributesTheServerLacks(t *testing.T) {
	var spec itemSpec
	for _, s := range itemSpecs {
		if s.itemtype == "Location" {
			spec = s
		}
	}
	configured := map[string]bool{"code": true}
	get := func(_ context.Context, p path.Path, target any) diag.Diagnostics {
		set := configured[p.String()]
		switch v := target.(type) {
		case *types.String:
			*v = types.StringNull()
			if set {
				*v = types.StringValue("x")
			}
		case *types.Int64:
			*v = types.Int64Null()
		case *types.Bool:
			*v = types.BoolNull()
		case *types.Float64:
			*v = types.Float64Null()
		}
		return nil
	}
	for major, wantErrors := range map[int]int{10: 1, 11: 0, 0: 0} {
		diags := spec.checkVersion(context.Background(), major, get)
		if len(diags.Errors()) != wantErrors {
			t.Errorf("GLPI %d: %d errors, want %d: %v", major, len(diags.Errors()), wantErrors, diags)
		}
	}
}

func TestParseItemIDAcceptsZeroOnlyForTheRootEntity(t *testing.T) {
	root, other := itemSpec{zeroID: true}, itemSpec{}
	if id, err := root.parseItemID("0"); err != nil || id != 0 {
		t.Errorf("root entity: %d, %v", id, err)
	}
	if _, err := other.parseItemID("0"); err == nil {
		t.Error("0 accepted for an itemtype without an ID 0")
	}
	if id, err := root.parseItemID("7"); err != nil || id != 7 {
		t.Errorf("ordinary ID on the root entity spec: %d, %v", id, err)
	}
}
