package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/glpi"
)

var _ datasource.DataSourceWithConfigure = (*itemDataSource)(nil)

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
			desc += " Set id or name; must match exactly one item."
		}
		if f.kind == kindInt {
			attrs[f.tf] = schema.Int64Attribute{Optional: optional, Computed: true, Description: desc}
		} else {
			attrs[f.tf] = schema.StringAttribute{Optional: optional, Computed: true, Description: desc}
		}
	}
	resp.Schema = schema.Schema{Description: "Look up a " + d.spec.desc, Attributes: attrs}
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
	if id.IsNull() == name.IsNull() {
		resp.Diagnostics.AddError("Invalid lookup", "Set exactly one of id or name.")
		return
	}

	var item glpi.Item
	var diags diag.Diagnostics
	if !id.IsNull() {
		item, diags = d.byID(ctx, id.ValueString())
	} else {
		item, diags = d.byName(ctx, name.ValueString())
	}
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
	resp.Diagnostics.Append(d.spec.setState(ctx, resp.State.SetAttribute, item)...)
}

func (d *itemDataSource) byID(ctx context.Context, raw string) (glpi.Item, diag.Diagnostics) {
	var diags diag.Diagnostics
	id, err := parseID(raw)
	if err != nil {
		diags.AddAttributeError(path.Root("id"), "Invalid ID", err.Error())
		return nil, diags
	}
	item, err := d.client.Get(ctx, d.spec.itemtype, id)
	if err != nil {
		diags.AddError("Reading "+d.spec.itemtype, err.Error())
		return nil, diags
	}
	if isTrashed(item) {
		diags.AddError("Not found", fmt.Sprintf("%s %d is in the GLPI trash bin.", d.spec.itemtype, id))
	}
	return item, diags
}

func (d *itemDataSource) byName(ctx context.Context, name string) (glpi.Item, diag.Diagnostics) {
	var diags diag.Diagnostics
	found, err := d.client.FindByName(ctx, d.spec.itemtype, name)
	if err != nil {
		diags.AddError("Searching "+d.spec.itemtype, err.Error())
		return nil, diags
	}
	switch len(found) {
	case 0:
		diags.AddError("Not found", fmt.Sprintf("No %s named %q.", d.spec.itemtype, name))
		return nil, diags
	case 1:
		return found[0], diags
	}
	diags.AddError("Ambiguous name", fmt.Sprintf("%d %s items are named %q; look up by id instead.", len(found), d.spec.itemtype, name))
	return nil, diags
}
