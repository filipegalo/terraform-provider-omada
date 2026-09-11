package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var _ resource.Resource = &ssidResource{}
var _ resource.ResourceWithConfigure = &ssidResource{}
var _ resource.ResourceWithImportState = &ssidResource{}

type ssidResource struct {
	client        *client.Client
	defaultSiteID string
}

// NewSSIDResource returns the omada_ssid resource implementation.
func NewSSIDResource() resource.Resource { return &ssidResource{} }

type ssidModel struct {
	ID          types.String `tfsdk:"id"`
	SiteID      types.String `tfsdk:"site_id"`
	WLANGroupID types.String `tfsdk:"wlan_group_id"`
	Name        types.String `tfsdk:"name"`
	Band        types.Int64  `tfsdk:"band"`
	Security    types.Int64  `tfsdk:"security"`
	PSK         types.String `tfsdk:"psk"`
	Broadcast   types.Bool   `tfsdk:"broadcast"`
	VLANEnable  types.Bool   `tfsdk:"vlan_enable"`
	VLANID      types.Int64  `tfsdk:"vlan_id"`
	Guest       types.Bool   `tfsdk:"guest"`
	Enable11r   types.Bool   `tfsdk:"enable_11r"`
	PMFMode     types.Int64  `tfsdk:"pmf_mode"`
}

func (r *ssidResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssid"
}

func (r *ssidResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An Omada wireless SSID. Updates preserve unmodelled controller settings and retain the current PSK unless a new one is configured.",
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site_id":       schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"wlan_group_id": schema.StringAttribute{Required: true, Description: "WLAN group containing the SSID.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name":          schema.StringAttribute{Required: true, Description: "Wireless network name."},
			"band":          schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(7), Description: "Radio band bitmask: 1 is 2.4GHz, 2 is 5GHz and 4 is 6GHz.", Validators: []validator.Int64{int64validator.Between(1, 7)}},
			"security":      schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(3), Description: "Security mode: 0 open or 3 WPA-PSK.", Validators: []validator.Int64{int64validator.OneOf(0, 3)}},
			"psk": schema.StringAttribute{
				Optional: true, Sensitive: true, WriteOnly: true,
				Description: "Pre-shared key. Write-only: it is sent to Omada but never persisted in Terraform state.",
			},
			"broadcast":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Broadcast the SSID."},
			"vlan_enable": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Tag traffic from this SSID with a VLAN."},
			"vlan_id":     schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1), Description: "802.1Q VLAN ID.", Validators: []validator.Int64{int64validator.Between(1, 4094)}},
			"guest":       schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Enable Omada guest-network restrictions."},
			"enable_11r":  schema.BoolAttribute{Optional: true, Computed: true, Description: "Enable 802.11r fast roaming."},
			"pmf_mode":    schema.Int64Attribute{Optional: true, Computed: true, Description: "Protected Management Frames controller enum.", Validators: []validator.Int64{int64validator.AtLeast(0)}},
		},
	}
}

func (r *ssidResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ssidResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config ssidModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := resourceSiteID(plan.SiteID, r.defaultSiteID)
	id, err := r.client.CreateSSID(ctx, siteID, plan.WLANGroupID.ValueString(), ssidConfig(plan, config.PSK.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Unable to create SSID", err.Error())
		return
	}
	plan.ID, plan.SiteID = types.StringValue(id), types.StringValue(siteID)
	found, err := r.client.FindSSID(ctx, siteID, plan.WLANGroupID.ValueString(), id)
	if err != nil || found == nil {
		resp.Diagnostics.AddError("Unable to read SSID after creation", readResultError(err, "SSID", id))
		return
	}
	applySSID(&plan, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ssidResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ssidModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.client.FindSSID(ctx, state.SiteID.ValueString(), state.WLANGroupID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read SSID", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	applySSID(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ssidResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, config, state ssidModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateSSID(ctx, state.SiteID.ValueString(), state.WLANGroupID.ValueString(), state.ID.ValueString(), ssidConfig(plan, config.PSK.ValueString())); err != nil {
		resp.Diagnostics.AddError("Unable to update SSID", err.Error())
		return
	}
	plan.ID, plan.SiteID = state.ID, state.SiteID
	found, err := r.client.FindSSID(ctx, state.SiteID.ValueString(), state.WLANGroupID.ValueString(), state.ID.ValueString())
	if err != nil || found == nil {
		resp.Diagnostics.AddError("Unable to read SSID after update", readResultError(err, "SSID", state.ID.ValueString()))
		return
	}
	applySSID(&plan, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ssidResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ssidModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSSID(ctx, state.SiteID.ValueString(), state.WLANGroupID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete SSID", err.Error())
	}
}

func (r *ssidResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected import ID \"site_id:wlan_group_id:ssid_id\", got: %q", req.ID))
		return
	}
	siteID, err := r.client.ResolveSite(ctx, parts[0])
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), siteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("wlan_group_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
}

func ssidConfig(model ssidModel, psk string) client.SSIDConfig {
	cfg := client.SSIDConfig{
		Name: model.Name.ValueString(), Band: model.Band.ValueInt64(), Security: model.Security.ValueInt64(), PSK: psk,
		Broadcast: model.Broadcast.ValueBool(), VLANEnable: model.VLANEnable.ValueBool(), VLANID: model.VLANID.ValueInt64(), Guest: model.Guest.ValueBool(),
	}
	if !model.Enable11r.IsNull() && !model.Enable11r.IsUnknown() {
		value := model.Enable11r.ValueBool()
		cfg.Enable11r = &value
	}
	if !model.PMFMode.IsNull() && !model.PMFMode.IsUnknown() {
		value := model.PMFMode.ValueInt64()
		cfg.PMFMode = &value
	}
	return cfg
}

func applySSID(state *ssidModel, ssid *client.SSID) {
	state.ID = types.StringValue(ssid.ID)
	state.Name = types.StringValue(ssid.Name)
	state.Band = types.Int64Value(ssid.Band)
	state.Security = types.Int64Value(ssid.Security)
	state.Broadcast = types.BoolValue(ssid.Broadcast)
	state.VLANEnable = types.BoolValue(ssid.VLANEnable)
	state.VLANID = types.Int64Value(ssid.VLANID)
	state.Guest = types.BoolValue(ssid.Guest)
	state.Enable11r = types.BoolValue(ssid.Enable11r)
	state.PMFMode = types.Int64Value(ssid.PMFMode)
}

func readResultError(err error, kind, id string) string {
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("%s %q was not returned by the controller", kind, id)
}
