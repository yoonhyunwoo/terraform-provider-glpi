package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/glpi"
)

func (f field) stateValue(v any) (attr.Value, error) {
	if f.kind == kindInt {
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
		return nil, fmt.Errorf("field %s: unexpected %T", f.api, v)
	}
	switch s := v.(type) {
	case nil:
		return types.StringValue(""), nil
	case string:
		return types.StringValue(html.UnescapeString(s)), nil
	case json.Number:
		return types.StringValue(s.String()), nil
	}
	return nil, fmt.Errorf("field %s: unexpected %T", f.api, v)
}

func (s itemSpec) apiInput(ctx context.Context, get func(context.Context, path.Path, any) diag.Diagnostics) (glpi.Item, diag.Diagnostics) {
	var diags diag.Diagnostics
	in := glpi.Item{}
	for _, f := range s.fields {
		if f.readOnly {
			continue
		}
		p := path.Root(f.tf)
		if f.kind == kindInt {
			var v types.Int64
			diags.Append(get(ctx, p, &v)...)
			if !v.IsNull() && !v.IsUnknown() {
				in[f.api] = v.ValueInt64()
			}
			continue
		}
		var v types.String
		diags.Append(get(ctx, p, &v)...)
		if !v.IsNull() && !v.IsUnknown() {
			in[f.api] = v.ValueString()
		}
	}
	return in, diags
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

func parseID(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != s {
		return 0, fmt.Errorf("%q is not a positive integer GLPI ID", s)
	}
	return n, nil
}
