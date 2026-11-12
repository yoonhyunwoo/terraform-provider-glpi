package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/glpi"
)

const (
	fieldsContainerItemtype = "PluginFieldsContainer"
	fieldsFieldItemtype     = "PluginFieldsField"
)

// scalarFieldTypes are the Fields plugin types whose value is one string in the item's own column.
// Dropdown, GLPI item and multiple-value fields use other input keys and are not managed.
var scalarFieldTypes = []string{"text", "textarea", "richtext", "number", "url", "date", "datetime", "yesno"}

type fieldsContainer struct {
	id        int64
	name      string
	label     string
	typ       string
	itemtypes []string
	active    bool
	fields    []fieldsField
}

type fieldsField struct {
	name      string
	label     string
	typ       string
	readOnly  bool
	mandatory bool
}

func loadFieldsContainer(ctx context.Context, c *glpi.Client, row glpi.Item) (fieldsContainer, error) {
	id, err := itemID(row)
	if err != nil {
		return fieldsContainer{}, err
	}
	ct := fieldsContainer{id: id, name: rowString(row["name"]), label: rowString(row["label"]), typ: rowString(row["type"])}
	ct.active = rowBool(row["is_active"])
	if raw := rowString(row["itemtypes"]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &ct.itemtypes); err != nil {
			return fieldsContainer{}, fmt.Errorf("container %d: unreadable itemtypes %q: %w", id, raw, err)
		}
	}
	all, err := c.List(ctx, fieldsFieldItemtype)
	if err != nil {
		return fieldsContainer{}, err
	}
	for _, f := range all {
		if cid, _ := strconv.ParseInt(rowString(f["plugin_fields_containers_id"]), 10, 64); cid != id {
			continue
		}
		ct.fields = append(ct.fields, fieldsField{
			name: rowString(f["name"]), label: rowString(f["label"]), typ: rowString(f["type"]),
			readOnly: rowBool(f["is_readonly"]), mandatory: rowBool(f["mandatory"]),
		})
	}
	sort.Slice(ct.fields, func(i, j int) bool { return ct.fields[i].name < ct.fields[j].name })
	return ct, nil
}

func (ct fieldsContainer) field(name string) (fieldsField, bool) {
	for _, f := range ct.fields {
		if f.name == name {
			return f, true
		}
	}
	return fieldsField{}, false
}

// valuesItemtype is the dynamic itemtype GLPI generates for the rows that hold a container's values.
func (ct fieldsContainer) valuesItemtype(itemtype string) string {
	return "PluginFields" + itemtype + ct.name
}

func rowString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case json.Number:
		return s.String()
	}
	return ""
}

func rowBool(v any) bool {
	s := rowString(v)
	return s != "" && s != "0"
}

// ---- data source ----

var fieldsFieldObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"name":      types.StringType,
	"label":     types.StringType,
	"type":      types.StringType,
	"read_only": types.BoolType,
	"mandatory": types.BoolType,
}}

var _ datasource.DataSourceWithConfigure = (*fieldsContainerDataSource)(nil)

type fieldsContainerDataSource struct{ client *glpi.Client }

type fieldsContainerModel struct {
	ID        types.String `tfsdk:"id"`
	Label     types.String `tfsdk:"label"`
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	ItemTypes types.List   `tfsdk:"item_types"`
	Active    types.Bool   `tfsdk:"active"`
	Fields    types.List   `tfsdk:"fields"`
}

func (d *fieldsContainerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin_fields_container"
}

func (d *fieldsContainerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		Description: "Look up a container of the GLPI Fields plugin and the fields it defines.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":         datasourceschema.StringAttribute{Optional: true, Computed: true, Description: "GLPI numeric ID. Set id or label."},
			"label":      datasourceschema.StringAttribute{Optional: true, Computed: true, Description: "Label of the container. Set id or label; the label must match exactly one container."},
			"name":       datasourceschema.StringAttribute{Computed: true, Description: "Internal name GLPI generated from the label."},
			"type":       datasourceschema.StringAttribute{Computed: true, Description: "Where the container appears: dom, tab or domtab."},
			"item_types": datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "GLPI itemtypes the container applies to."},
			"active":     datasourceschema.BoolAttribute{Computed: true, Description: "Whether the container is active."},
			"fields": datasourceschema.ListNestedAttribute{
				Computed:    true,
				Description: "Fields of the container, ordered by name. Only the types text, textarea, richtext, number, url, date, datetime and yesno can be managed with glpi_plugin_fields_values.",
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"name":      datasourceschema.StringAttribute{Computed: true, Description: "Key GLPI uses for the field's value."},
					"label":     datasourceschema.StringAttribute{Computed: true, Description: "Label of the field."},
					"type":      datasourceschema.StringAttribute{Computed: true, Description: "Field type."},
					"read_only": datasourceschema.BoolAttribute{Computed: true, Description: "Whether GLPI ignores writes to the field."},
					"mandatory": datasourceschema.BoolAttribute{Computed: true, Description: "Whether the field must have a value."},
				}},
			},
		},
	}
}

func (d *fieldsContainerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *fieldsContainerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg fieldsContainerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.ID.IsNull() == cfg.Label.IsNull() {
		resp.Diagnostics.AddError("Invalid lookup", "Set exactly one of id or label.")
		return
	}
	row, err := d.findRow(ctx, cfg)
	if err != nil {
		resp.Diagnostics.AddError("Reading "+fieldsContainerItemtype, err.Error())
		return
	}
	ct, err := loadFieldsContainer(ctx, d.client, row)
	if err != nil {
		resp.Diagnostics.AddError("Reading "+fieldsContainerItemtype, err.Error())
		return
	}
	var diags diag.Diagnostics
	model := fieldsContainerModel{
		ID: types.StringValue(strconv.FormatInt(ct.id, 10)), Label: types.StringValue(ct.label),
		Name: types.StringValue(ct.name), Type: types.StringValue(ct.typ), Active: types.BoolValue(ct.active),
	}
	model.ItemTypes, diags = types.ListValueFrom(ctx, types.StringType, ct.itemtypes)
	resp.Diagnostics.Append(diags...)
	objs := make([]attr.Value, 0, len(ct.fields))
	for _, f := range ct.fields {
		obj, od := types.ObjectValue(fieldsFieldObjectType.AttrTypes, map[string]attr.Value{
			"name": types.StringValue(f.name), "label": types.StringValue(f.label), "type": types.StringValue(f.typ),
			"read_only": types.BoolValue(f.readOnly), "mandatory": types.BoolValue(f.mandatory),
		})
		resp.Diagnostics.Append(od...)
		objs = append(objs, obj)
	}
	model.Fields, diags = types.ListValue(fieldsFieldObjectType, objs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (d *fieldsContainerDataSource) findRow(ctx context.Context, cfg fieldsContainerModel) (glpi.Item, error) {
	if !cfg.ID.IsNull() {
		id, err := parseID(cfg.ID.ValueString())
		if err != nil {
			return nil, err
		}
		return d.client.Get(ctx, fieldsContainerItemtype, id)
	}
	rows, err := d.client.List(ctx, fieldsContainerItemtype)
	if err != nil {
		return nil, err
	}
	var found []glpi.Item
	for _, r := range rows {
		if rowString(r["label"]) == cfg.Label.ValueString() {
			found = append(found, r)
		}
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("no container labelled %q", cfg.Label.ValueString())
	case 1:
		return found[0], nil
	}
	return nil, fmt.Errorf("%d containers are labelled %q; look up by id instead", len(found), cfg.Label.ValueString())
}

// ---- resource ----

var (
	_ resource.ResourceWithConfigure   = (*fieldsValuesResource)(nil)
	_ resource.ResourceWithImportState = (*fieldsValuesResource)(nil)
)

type fieldsValuesResource struct{ client *glpi.Client }

type fieldsValuesModel struct {
	ID          types.String `tfsdk:"id"`
	ItemType    types.String `tfsdk:"item_type"`
	ItemID      types.Int64  `tfsdk:"item_id"`
	ContainerID types.Int64  `tfsdk:"container_id"`
	Values      types.Map    `tfsdk:"values"`
}

func (r *fieldsValuesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin_fields_values"
}

func (r *fieldsValuesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		Description: "Values of GLPI Fields plugin fields on one item. Needs GLPI 11.0.2 or later and Fields 1.23.3 or later, which accept the values through the item's own update. Only the fields listed in values are managed; other fields of the container keep their values. Destroying the resource empties the managed fields.",
		Attributes: map[string]resourceschema.Attribute{
			"id": resourceschema.StringAttribute{
				Computed:      true,
				Description:   "Import ID in the form itemtype/item_id/container_id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"item_type": resourceschema.StringAttribute{
				Required:      true,
				Description:   "GLPI itemtype of the item that holds the values, for example Computer.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"item_id": resourceschema.Int64Attribute{
				Required:      true,
				Description:   "ID of the item that holds the values.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"container_id": resourceschema.Int64Attribute{
				Required:      true,
				Description:   "ID of the Fields container, from the glpi_plugin_fields_container data source.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"values": resourceschema.MapAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Field name to value. Dates are YYYY-MM-DD, date times YYYY-MM-DD HH:MM:SS and yes/no fields 0 or 1. GLPI rewrites some numbers, for example 12,5 to 12.5, so write them the way GLPI stores them.",
			},
		},
	}
}

func (r *fieldsValuesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func fieldsValuesID(itemtype string, itemID, containerID int64) string {
	return itemtype + "/" + strconv.FormatInt(itemID, 10) + "/" + strconv.FormatInt(containerID, 10)
}

func (r *fieldsValuesResource) container(ctx context.Context, containerID int64, itemtype string) (fieldsContainer, error) {
	row, err := r.client.Get(ctx, fieldsContainerItemtype, containerID)
	if err != nil {
		return fieldsContainer{}, err
	}
	ct, err := loadFieldsContainer(ctx, r.client, row)
	if err != nil {
		return fieldsContainer{}, err
	}
	if !slices.Contains(ct.itemtypes, itemtype) {
		return fieldsContainer{}, fmt.Errorf("container %q applies to %v, not %s", ct.label, ct.itemtypes, itemtype)
	}
	return ct, nil
}

// checkManageable rejects keys GLPI would silently drop: unknown fields, read-only fields and field
// types that need other input keys.
func (ct fieldsContainer) checkManageable(keys []string) error {
	for _, k := range keys {
		f, ok := ct.field(k)
		switch {
		case !ok:
			return fmt.Errorf("container %q has no field %q", ct.label, k)
		case f.readOnly:
			return fmt.Errorf("field %q of container %q is read-only in GLPI", k, ct.label)
		case !slices.Contains(scalarFieldTypes, f.typ):
			return fmt.Errorf("field %q has type %q, which the provider does not manage; supported types are %s", k, f.typ, strings.Join(scalarFieldTypes, ", "))
		}
	}
	return nil
}

func (ct fieldsContainer) emptyValue(name string) string {
	if f, ok := ct.field(name); ok && f.typ == "yesno" {
		return "0"
	}
	return ""
}

func (r *fieldsValuesResource) write(ctx context.Context, itemtype string, itemID int64, ct fieldsContainer, values map[string]string) error {
	input := glpi.Item{"c_id": ct.id}
	for k, v := range values {
		input[k] = v
	}
	return r.client.Update(ctx, itemtype, itemID, input)
}

// readValues returns the stored values of the given fields, with GLPI's null as the empty string.
func (r *fieldsValuesResource) readValues(ctx context.Context, itemtype string, itemID int64, ct fieldsContainer, keys []string) (map[string]string, error) {
	rows, err := r.client.SubItems(ctx, itemtype, itemID, ct.valuesItemtype(itemtype))
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(keys))
	var row glpi.Item
	if len(rows) > 0 {
		row = rows[0]
	}
	for _, k := range keys {
		out[k] = rowString(row[k])
	}
	return out, nil
}

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// apply writes the planned values, empties fields dropped since the prior state and verifies GLPI
// stored what was planned.
func (r *fieldsValuesResource) apply(ctx context.Context, plan fieldsValuesModel, prior map[string]string) (map[string]string, error) {
	itemtype, itemID, cid := plan.ItemType.ValueString(), plan.ItemID.ValueInt64(), plan.ContainerID.ValueInt64()
	ct, err := r.container(ctx, cid, itemtype)
	if err != nil {
		return nil, err
	}
	want := map[string]string{}
	if d := plan.Values.ElementsAs(ctx, &want, false); d.HasError() {
		return nil, fmt.Errorf("reading values: %s", d.Errors()[0].Detail())
	}
	if err := ct.checkManageable(mapKeys(want)); err != nil {
		return nil, err
	}
	send := maps.Clone(want)
	for k := range prior {
		if _, kept := want[k]; !kept {
			send[k] = ct.emptyValue(k)
		}
	}
	if err := r.write(ctx, itemtype, itemID, ct, send); err != nil {
		return nil, err
	}
	got, err := r.readValues(ctx, itemtype, itemID, ct, mapKeys(want))
	if err != nil {
		return nil, err
	}
	for _, k := range mapKeys(want) {
		if got[k] != want[k] {
			return nil, fmt.Errorf("field %q: GLPI stored %q instead of %q", k, got[k], want[k])
		}
	}
	return got, nil
}

func (r *fieldsValuesResource) model(ctx context.Context, base fieldsValuesModel, values map[string]string) (fieldsValuesModel, diag.Diagnostics) {
	base.ID = types.StringValue(fieldsValuesID(base.ItemType.ValueString(), base.ItemID.ValueInt64(), base.ContainerID.ValueInt64()))
	mv, d := types.MapValueFrom(ctx, types.StringType, values)
	base.Values = mv
	return base, d
}

func (r *fieldsValuesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan fieldsValuesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.apply(ctx, plan, nil)
	if err != nil {
		resp.Diagnostics.AddError("Writing Fields values", err.Error())
		return
	}
	m, d := r.model(ctx, plan, got)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *fieldsValuesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior fieldsValuesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	before := map[string]string{}
	resp.Diagnostics.Append(prior.Values.ElementsAs(ctx, &before, false)...)
	got, err := r.apply(ctx, plan, before)
	if err != nil {
		resp.Diagnostics.AddError("Writing Fields values", err.Error())
		return
	}
	m, d := r.model(ctx, plan, got)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *fieldsValuesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state fieldsValuesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	itemtype, itemID, cid := state.ItemType.ValueString(), state.ItemID.ValueInt64(), state.ContainerID.ValueInt64()
	item, err := r.client.Get(ctx, itemtype, itemID)
	if isGone(item, err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading "+itemtype, err.Error())
		return
	}
	ct, err := r.container(ctx, cid, itemtype)
	if err != nil {
		resp.Diagnostics.AddError("Reading Fields container", err.Error())
		return
	}
	keys := map[string]string{}
	resp.Diagnostics.Append(state.Values.ElementsAs(ctx, &keys, false)...)
	got, err := r.readValues(ctx, itemtype, itemID, ct, mapKeys(keys))
	if err != nil {
		resp.Diagnostics.AddError("Reading Fields values", err.Error())
		return
	}
	m, d := r.model(ctx, state, got)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *fieldsValuesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state fieldsValuesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	itemtype, itemID := state.ItemType.ValueString(), state.ItemID.ValueInt64()
	item, err := r.client.Get(ctx, itemtype, itemID)
	if isGone(item, err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading "+itemtype, err.Error())
		return
	}
	ct, err := r.container(ctx, state.ContainerID.ValueInt64(), itemtype)
	if err != nil {
		resp.Diagnostics.AddError("Reading Fields container", err.Error())
		return
	}
	managed := map[string]string{}
	resp.Diagnostics.Append(state.Values.ElementsAs(ctx, &managed, false)...)
	emptied := map[string]string{}
	for k := range managed {
		emptied[k] = ct.emptyValue(k)
	}
	if err := r.write(ctx, itemtype, itemID, ct, emptied); err != nil {
		resp.Diagnostics.AddError("Emptying Fields values", err.Error())
	}
}

func (r *fieldsValuesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 {
		resp.Diagnostics.AddError("Invalid import ID", "Use itemtype/item_id/container_id, for example Computer/43/3.")
		return
	}
	itemID, err1 := parseID(parts[1])
	cid, err2 := parseID(parts[2])
	if err1 != nil || err2 != nil {
		resp.Diagnostics.AddError("Invalid import ID", "item_id and container_id must be positive integers.")
		return
	}
	ct, err := r.container(ctx, cid, parts[0])
	if err != nil {
		resp.Diagnostics.AddError("Reading Fields container", err.Error())
		return
	}
	names := make([]string, 0, len(ct.fields))
	for _, f := range ct.fields {
		if slices.Contains(scalarFieldTypes, f.typ) && !f.readOnly {
			names = append(names, f.name)
		}
	}
	all, err := r.readValues(ctx, parts[0], itemID, ct, names)
	if err != nil {
		resp.Diagnostics.AddError("Reading Fields values", err.Error())
		return
	}
	set := map[string]string{}
	for k, v := range all {
		if v != "" && v != ct.emptyValue(k) {
			set[k] = v
		}
	}
	model := fieldsValuesModel{ItemType: types.StringValue(parts[0]), ItemID: types.Int64Value(itemID), ContainerID: types.Int64Value(cid)}
	m, d := r.model(ctx, model, set)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
