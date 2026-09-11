package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var _ resource.Resource = &wlanGroupResource{}
var _ resource.ResourceWithConfigure = &wlanGroupResource{}
var _ resource.ResourceWithImportState = &wlanGroupResource{}

type wlanGroupResource struct {
	client        *client.Client
	defaultSiteID string
}

// NewWLANGroupResource returns the omada_wlan_group resource implementation.
func NewWLANGroupResource() resource.Resource { return &wlanGroupResource{} }

type wlanGroupModel struct {
	ID      types.String `tfsdk:"id"`
	SiteID  types.String `tfsdk:"site_id"`
	Name    types.String `tfsdk:"name"`
	Primary types.Bool   `tfsdk:"primary"`
}

func (r *wlanGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wlan_group"
}

func (r *wlanGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An Omada WLAN group containing one or more SSIDs.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site_id": schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"name":    schema.StringAttribute{Required: true, Description: "WLAN group name."},
			"primary": schema.BoolAttribute{Computed: true, Description: "Whether this is the controller's primary WLAN group."},
		},
	}
}

func (r *wlanGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*client.ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *client.ProviderData, got: %T", req.ProviderData))
		return
	}
	r.client, r.defaultSiteID = data.Client, data.SiteID
}

func (r *wlanGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wlanGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := resourceSiteID(plan.SiteID, r.defaultSiteID)
	id, err := r.client.CreateWLANGroup(ctx, siteID, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create WLAN group", err.Error())
		return
	}
	plan.ID, plan.SiteID = types.StringValue(id), types.StringValue(siteID)
	found, err := r.client.FindWLANGroup(ctx, siteID, id)
	if err != nil || found == nil {
		resp.Diagnostics.AddError("Unable to read WLAN group after creation", readResultError(err, "WLAN group", id))
		return
	}
	applyWLANGroup(&plan, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wlanGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state wlanGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.client.FindWLANGroup(ctx, state.SiteID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read WLAN group", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	applyWLANGroup(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *wlanGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state wlanGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateWLANGroup(ctx, state.SiteID.ValueString(), state.ID.ValueString(), plan.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to update WLAN group", err.Error())
		return
	}
	plan.ID, plan.SiteID, plan.Primary = state.ID, state.SiteID, state.Primary
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wlanGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state wlanGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteWLANGroup(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete WLAN group", err.Error())
	}
}

func (r *wlanGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	site, id, ok := strings.Cut(req.ID, ":")
	if !ok || site == "" || id == "" {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected import ID \"site_id:wlan_group_id\", got: %q", req.ID))
		return
	}
	siteID, err := r.client.ResolveSite(ctx, site)
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), siteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func applyWLANGroup(state *wlanGroupModel, group *client.WLANGroup) {
	state.ID = types.StringValue(group.ID)
	state.Name = types.StringValue(group.Name)
	state.Primary = types.BoolValue(group.Primary)
}
