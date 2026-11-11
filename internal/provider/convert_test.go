package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/glpi"
)

func TestStateValueNormalisesGLPIQuirks(t *testing.T) {
	str := field{api: "serial", kind: kindString}
	num := field{api: "locations_id", kind: kindInt}
	cases := []struct {
		name string
		f    field
		in   any
		want attr.Value
	}{
		{"null string becomes empty", str, nil, types.StringValue("")},
		{"string passes through", str, "SN1", types.StringValue("SN1")},
		{"number in string field", str, json.Number("7"), types.StringValue("7")},
		{"GLPI 10.0 stores &, < and > as entities", str, "R&#38;D &#60;team&#62;", types.StringValue("R&D <team>")},
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
