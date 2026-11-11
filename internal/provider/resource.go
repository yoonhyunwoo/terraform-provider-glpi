package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/glpi"
)

var (
	_ resource.ResourceWithConfigure   = (*itemResource)(nil)
	_ resource.ResourceWithImportState = (*itemResource)(nil)
)

// itemResource is one generic CRUD implementation shared by every itemtype in itemSpecs.
type itemResource struct {
	spec   itemSpec
	client *glpi.Client
}

func (r *itemResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.spec.tfName
}

func (r *itemResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "GLPI numeric ID.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for _, f := range r.spec.fields {
		switch {
		case f.kind == kindInt && f.readOnly:
			attrs[f.tf] = schema.Int64Attribute{Computed: true, Description: f.desc}
		case f.kind == kindInt:
			attrs[f.tf] = schema.Int64Attribute{
				Required: f.required, Optional: !f.required, Computed: !f.required,
				Description:   f.desc,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			}
		case f.readOnly:
			attrs[f.tf] = schema.StringAttribute{Computed: true, Description: f.desc}
		default:
			attrs[f.tf] = schema.StringAttribute{
				Required: f.required, Optional: !f.required, Computed: !f.required,
				Description:   f.desc,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			}
		}
	}
	resp.Schema = schema.Schema{
		Description: r.spec.desc + " Optional attributes that are removed from the configuration keep their last value in GLPI.",
		Attributes:  attrs,
	}
}

func (r *itemResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*glpi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *itemResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	input, diags := r.spec.apiInput(ctx, req.Plan.GetAttribute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.client.Create(ctx, r.spec.itemtype, input)
	if err != nil {
		resp.Diagnostics.AddError("Creating "+r.spec.itemtype, err.Error())
		return
	}
	// Persist the ID first so a failing read-back leaves a tainted resource instead of an orphan.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strconv.FormatInt(id, 10))...)
	r.refresh(ctx, id, &resp.State, &resp.Diagnostics)
}

func (r *itemResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var idStr types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &idStr)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(idStr.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid ID", err.Error())
		return
	}
	item, err := r.client.Get(ctx, r.spec.itemtype, id)
	if glpi.IsNotFound(err) || (err == nil && isTrashed(item)) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading "+r.spec.itemtype, err.Error())
		return
	}
	resp.Diagnostics.Append(r.spec.setState(ctx, resp.State.SetAttribute, id, item)...)
}

func (r *itemResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var idStr types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &idStr)...)
	input, diags := r.spec.apiInput(ctx, req.Plan.GetAttribute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(idStr.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid ID", err.Error())
		return
	}
	if err := r.client.Update(ctx, r.spec.itemtype, id, input); err != nil {
		resp.Diagnostics.AddError("Updating "+r.spec.itemtype, err.Error())
		return
	}
	r.refresh(ctx, id, &resp.State, &resp.Diagnostics)
}

func (r *itemResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var idStr types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &idStr)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(idStr.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid ID", err.Error())
		return
	}
	if err := r.client.Delete(ctx, r.spec.itemtype, id); err != nil && !glpi.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting "+r.spec.itemtype, err.Error())
	}
}

func (r *itemResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := strconv.ParseInt(req.ID, 10, 64); err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "GLPI IDs are integers: "+req.ID)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// refresh reads the item back so computed attributes reflect what GLPI stored.
func (r *itemResource) refresh(ctx context.Context, id int64, state *tfsdk.State, diags *diag.Diagnostics) {
	item, err := r.client.Get(ctx, r.spec.itemtype, id)
	if err != nil {
		diags.AddError("Reading "+r.spec.itemtype+" after write", err.Error())
		return
	}
	diags.Append(r.spec.setState(ctx, state.SetAttribute, id, item)...)
}
