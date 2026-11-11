package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/glpi"
)

var _ datasource.DataSourceWithConfigure = (*itemDataSource)(nil)

// itemDataSource looks one item up by id or by exact name.
type itemDataSource struct {
	spec   itemSpec
	client *glpi.Client
}

func (d *itemDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.spec.tfName
}

func (d *itemDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{Optional: true, Computed: true, Description: "GLPI numeric ID. Set id or name."},
	}
	for _, f := range d.spec.fields {
		optional := f.tf == "name"
		desc := f.desc
		if optional {
			desc += " Set id or name; the name must match exactly one item."
		}
		if f.kind == kindInt {
			attrs[f.tf] = schema.Int64Attribute{Optional: optional, Computed: true, Description: desc}
		} else {
			attrs[f.tf] = schema.StringAttribute{Optional: optional, Computed: true, Description: desc}
		}
	}
	resp.Schema = schema.Schema{Description: "Looks up " + d.spec.desc, Attributes: attrs}
}

func (d *itemDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*glpi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *itemDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var id, name types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &id)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("name"), &name)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hasID, hasName := !id.IsNull(), !name.IsNull()
	if hasID == hasName {
		resp.Diagnostics.AddError("Invalid lookup", "Set exactly one of id or name.")
		return
	}

	var item glpi.Item
	var itemID int64
	if hasID {
		n, err := parseID(id.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("id"), "Invalid ID", err.Error())
			return
		}
		if item, err = d.client.Get(ctx, d.spec.itemtype, n); err != nil {
			resp.Diagnostics.AddError("Reading "+d.spec.itemtype, err.Error())
			return
		}
		if isTrashed(item) {
			resp.Diagnostics.AddError("Not found", fmt.Sprintf("%s %d is in the GLPI trash bin.", d.spec.itemtype, n))
			return
		}
		itemID = n
	} else {
		found, err := d.client.FindByName(ctx, d.spec.itemtype, name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Searching "+d.spec.itemtype, err.Error())
			return
		}
		switch len(found) {
		case 0:
			resp.Diagnostics.AddError("Not found", fmt.Sprintf("No %s named %q.", d.spec.itemtype, name.ValueString()))
			return
		case 1:
			item = found[0]
		default:
			resp.Diagnostics.AddError("Ambiguous name", fmt.Sprintf("%d %s items are named %q; look up by id instead.", len(found), d.spec.itemtype, name.ValueString()))
			return
		}
		num, ok := item["id"].(json.Number)
		if !ok {
			resp.Diagnostics.AddError("Unexpected GLPI response", "search result has no numeric id")
			return
		}
		n, err := num.Int64()
		if err != nil {
			resp.Diagnostics.AddError("Unexpected GLPI response", err.Error())
			return
		}
		itemID = n
	}
	resp.Diagnostics.Append(d.spec.setState(ctx, resp.State.SetAttribute, itemID, item)...)
}
