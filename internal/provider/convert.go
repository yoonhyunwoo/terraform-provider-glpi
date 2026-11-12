package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/glpi"
)

func (f field) stateValue(v any) (attr.Value, error) {
	switch f.kind {
	case kindInt:
		return f.intValue(v)
	case kindBool:
		return f.boolValue(v)
	case kindFloat:
		return f.floatValue(v)
	}
	switch s := v.(type) {
	case nil:
		return types.StringValue(""), nil
	case string:
		return types.StringValue(s), nil
	case json.Number:
		return types.StringValue(s.String()), nil
	}
	return nil, f.unexpected(v)
}

func (f field) unexpected(v any) error { return fmt.Errorf("field %s: unexpected %T", f.api, v) }

func (f field) intValue(v any) (attr.Value, error) {
	switch n := v.(type) {
	case nil:
		return types.Int64Value(0), nil
	case json.Number:
		i, err := n.Int64()
		return types.Int64Value(i), err
	case string:
		if n == "" {
			return types.Int64Value(0), nil
		}
		i, err := strconv.ParseInt(n, 10, 64)
		return types.Int64Value(i), err
	}
	return nil, f.unexpected(v)
}

func (f field) boolValue(v any) (attr.Value, error) {
	switch b := v.(type) {
	case nil:
		return types.BoolValue(false), nil
	case bool:
		return types.BoolValue(b), nil
	case json.Number:
		i, err := b.Int64()
		return types.BoolValue(i != 0), err
	case string:
		return types.BoolValue(b != "" && b != "0"), nil
	}
	return nil, f.unexpected(v)
}

func (f field) floatValue(v any) (attr.Value, error) {
	switch n := v.(type) {
	case nil:
		return types.Float64Value(0), nil
	case json.Number:
		x, err := n.Float64()
		return types.Float64Value(x), err
	case string:
		if n == "" {
			return types.Float64Value(0), nil
		}
		x, err := strconv.ParseFloat(n, 64)
		return types.Float64Value(x), err
	}
	return nil, f.unexpected(v)
}

func (s itemSpec) apiInput(ctx context.Context, get func(context.Context, path.Path, any) diag.Diagnostics) (glpi.Item, diag.Diagnostics) {
	var diags diag.Diagnostics
	in := glpi.Item{}
	for _, f := range s.fields {
		if f.readOnly {
			continue
		}
		p := path.Root(f.tf)
		switch f.kind {
		case kindInt:
			var v types.Int64
			diags.Append(get(ctx, p, &v)...)
			if !v.IsNull() && !v.IsUnknown() {
				in[f.api] = v.ValueInt64()
			}
		case kindBool:
			var v types.Bool
			diags.Append(get(ctx, p, &v)...)
			if !v.IsNull() && !v.IsUnknown() {
				in[f.api] = boolToInt(v.ValueBool())
			}
		case kindFloat:
			var v types.Float64
			diags.Append(get(ctx, p, &v)...)
			if !v.IsNull() && !v.IsUnknown() {
				in[f.api] = v.ValueFloat64()
			}
		default:
			var v types.String
			diags.Append(get(ctx, p, &v)...)
			if !v.IsNull() && !v.IsUnknown() {
				in[f.api] = v.ValueString()
			}
		}
	}
	if s.prepare != nil {
		s.prepare(in)
	}
	return in, diags
}

// checkVersion rejects attributes the configuration sets but the server's GLPI version has no column
// for. GLPI would ignore the input and the value would never read back. A server that does not report
// its version, major 0, is given the benefit of the doubt.
func (s itemSpec) checkVersion(ctx context.Context, major int, get func(context.Context, path.Path, any) diag.Diagnostics) diag.Diagnostics {
	var diags diag.Diagnostics
	for _, f := range s.fields {
		if f.minGLPI == 0 || major == 0 || major >= f.minGLPI {
			continue
		}
		var v attr.Value
		switch f.kind {
		case kindInt:
			var x types.Int64
			diags.Append(get(ctx, path.Root(f.tf), &x)...)
			v = x
		case kindBool:
			var x types.Bool
			diags.Append(get(ctx, path.Root(f.tf), &x)...)
			v = x
		case kindFloat:
			var x types.Float64
			diags.Append(get(ctx, path.Root(f.tf), &x)...)
			v = x
		default:
			var x types.String
			diags.Append(get(ctx, path.Root(f.tf), &x)...)
			v = x
		}
		if !v.IsNull() {
			diags.AddAttributeError(path.Root(f.tf), "Not supported by this GLPI",
				fmt.Sprintf("%s needs GLPI %d or later; the server runs GLPI %d.", f.tf, f.minGLPI, major))
		}
	}
	return diags
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func itemID(item glpi.Item) (int64, error) {
	n, ok := item["id"].(json.Number)
	if !ok {
		return 0, errors.New("GLPI response has no numeric id")
	}
	return n.Int64()
}

func (s itemSpec) setState(ctx context.Context, set func(context.Context, path.Path, any) diag.Diagnostics, item glpi.Item) diag.Diagnostics {
	id, err := itemID(item)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unexpected GLPI response", err.Error())
		return diags
	}
	diags := set(ctx, path.Root("id"), strconv.FormatInt(id, 10))
	for _, f := range s.fields {
		v, err := f.stateValue(item[f.api])
		if err != nil {
			diags.AddError("Unexpected GLPI response", err.Error())
			continue
		}
		diags.Append(set(ctx, path.Root(f.tf), v)...)
	}
	return diags
}

func isTrashed(item glpi.Item) bool {
	switch v := item["is_deleted"].(type) {
	case json.Number:
		i, _ := v.Int64()
		return i != 0
	case string:
		return v != "" && v != "0"
	case bool:
		return v
	}
	return false
}

func isGone(item glpi.Item, err error) bool {
	return glpi.IsNotFound(err) || (err == nil && isTrashed(item))
}

// parseItemID is parseID that also accepts "0" for the itemtype whose first item has that ID.
func (s itemSpec) parseItemID(raw string) (int64, error) {
	if s.zeroID && raw == "0" {
		return 0, nil
	}
	return parseID(raw)
}

func parseID(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != s {
		return 0, fmt.Errorf("%q is not a positive integer GLPI ID", s)
	}
	return n, nil
}
