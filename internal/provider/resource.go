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
	id, ok := stateID(ctx, req.State, &resp.Diagnostics)
	if !ok {
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
	resp.Diagnostics.Append(r.spec.setState(ctx, resp.State.SetAttribute, item)...)
}

func (r *itemResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	id, ok := stateID(ctx, req.State, &resp.Diagnostics)
	if !ok {
		return
	}
	input, diags := r.spec.apiInput(ctx, req.Plan.GetAttribute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Update(ctx, r.spec.itemtype, id, input); err != nil {
		resp.Diagnostics.AddError("Updating "+r.spec.itemtype, err.Error())
		return
	}
	r.refresh(ctx, id, &resp.State, &resp.Diagnostics)
}

func (r *itemResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	id, ok := stateID(ctx, req.State, &resp.Diagnostics)
	if !ok {
		return
	}
	if err := r.client.Delete(ctx, r.spec.itemtype, id); err != nil {
		// GLPI answers a delete of a trashed item with a 400. Gone from the trash bin's point of view is gone.
		if item, gerr := r.client.Get(ctx, r.spec.itemtype, id); glpi.IsNotFound(gerr) || (gerr == nil && isTrashed(item)) {
			return
		}
		resp.Diagnostics.AddError("Deleting "+r.spec.itemtype, err.Error())
	}
}

func (r *itemResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := parseID(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// stateID reads and validates the item ID held in state.
func stateID(ctx context.Context, state tfsdk.State, diags *diag.Diagnostics) (int64, bool) {
	var s types.String
	diags.Append(state.GetAttribute(ctx, path.Root("id"), &s)...)
	if diags.HasError() {
		return 0, false
	}
	id, err := parseID(s.ValueString())
	if err != nil {
		diags.AddError("Invalid ID", err.Error())
		return 0, false
	}
	return id, true
}

// refresh reads the item back so computed attributes reflect what GLPI stored.
func (r *itemResource) refresh(ctx context.Context, id int64, state *tfsdk.State, diags *diag.Diagnostics) {
	item, err := r.client.Get(ctx, r.spec.itemtype, id)
	if err != nil {
		diags.AddError("Reading "+r.spec.itemtype+" after write", err.Error())
		return
	}
	diags.Append(r.spec.setState(ctx, state.SetAttribute, item)...)
}
