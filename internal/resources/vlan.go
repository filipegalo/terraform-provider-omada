package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var (
	_ resource.Resource                   = &vlanResource{}
	_ resource.ResourceWithConfigure      = &vlanResource{}
	_ resource.ResourceWithImportState    = &vlanResource{}
	_ resource.ResourceWithValidateConfig = &vlanResource{}
)

type vlanResource struct {
	client        *client.Client
	defaultSiteID string
}

// NewVLANResource returns the omada_vlan resource implementation.
func NewVLANResource() resource.Resource { return &vlanResource{} }

type vlanModel struct {
	ID             types.String `tfsdk:"id"`
	SiteID         types.String `tfsdk:"site_id"`
	Name           types.String `tfsdk:"name"`
	VLANID         types.Int64  `tfsdk:"vlan_id"`
	DeviceMAC      types.String `tfsdk:"device_mac"`
	GatewaySubnet  types.String `tfsdk:"gateway_subnet"`
	DHCPEnabled    types.Bool   `tfsdk:"dhcp_enabled"`
	DHCPRangeStart types.String `tfsdk:"dhcp_range_start"`
	DHCPRangeEnd   types.String `tfsdk:"dhcp_range_end"`
	DHCPDNSMode    types.String `tfsdk:"dhcp_dns_mode"`
	DHCPPrimaryDNS types.String `tfsdk:"dhcp_primary_dns"`
	DHCPLeaseTime  types.Int64  `tfsdk:"dhcp_lease_time"`
	Isolation      types.Bool   `tfsdk:"isolation"`
}

// ValidateConfig enforces the rules the controller has but the schema cannot
// express attribute by attribute, so they surface during plan instead of
// halfway through an apply -- and in update as well as create, which the old
// check inside Create missed.
func (r *vlanResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config vlanModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// A null dhcp_enabled still means enabled: defaults are not applied yet at
	// config-validation time, and this attribute defaults to true.
	enabled := config.DHCPEnabled.IsNull() || config.DHCPEnabled.ValueBool()
	if enabled && !config.DHCPEnabled.IsUnknown() {
		for _, attr := range []struct {
			name  string
			value types.String
		}{
			{"dhcp_range_start", config.DHCPRangeStart},
			{"dhcp_range_end", config.DHCPRangeEnd},
		} {
			if attr.value.IsNull() {
				resp.Diagnostics.AddAttributeError(path.Root(attr.name), "Missing DHCP pool",
					fmt.Sprintf("%s is required when dhcp_enabled is true.", attr.name))
			}
		}
	}
	if config.DHCPDNSMode.ValueString() == "manual" && config.DHCPPrimaryDNS.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("dhcp_primary_dns"), "Missing DNS server",
			"dhcp_primary_dns is required when dhcp_dns_mode is \"manual\".")
	}
}

func (r *vlanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vlan"
}

func (r *vlanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A gateway-backed VLAN interface on an Omada Controller site, including its IPv4 gateway and DHCP scope.",
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site_id":          schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"name":             schema.StringAttribute{Required: true, Description: "Display name for the VLAN."},
			"vlan_id":          schema.Int64Attribute{Required: true, Description: "IEEE 802.1Q VLAN ID (1 through 4094).", Validators: []validator.Int64{int64validator.Between(1, 4094)}, PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"device_mac":       schema.StringAttribute{Required: true, Description: "MAC address of the Omada gateway that owns this VLAN interface.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"gateway_subnet":   schema.StringAttribute{Required: true, Description: "Gateway IPv4 address and CIDR, for example 192.168.20.1/24."},
			"dhcp_enabled":     schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Enable the gateway DHCP server for this VLAN."},
			"dhcp_range_start": schema.StringAttribute{Optional: true, Description: "First address in the DHCP pool; required when dhcp_enabled is true."},
			"dhcp_range_end":   schema.StringAttribute{Optional: true, Description: "Last address in the DHCP pool; required when dhcp_enabled is true."},
			"dhcp_dns_mode":    schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("auto"), Description: "DHCP DNS mode: auto or manual.", Validators: []validator.String{stringvalidator.OneOf("auto", "manual")}},
			"dhcp_primary_dns": schema.StringAttribute{Optional: true, Description: "Primary DNS server when dhcp_dns_mode is manual."},
			"dhcp_lease_time":  schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1440), Description: "DHCP lease time in minutes."},
			"isolation":        schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Enable inter-VLAN isolation."},
		},
	}
}

func (r *vlanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *vlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vlanModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := plan.SiteID.ValueString()
	if plan.SiteID.IsNull() || plan.SiteID.IsUnknown() {
		siteID = r.defaultSiteID
	}
	id, err := r.client.CreateVLAN(ctx, siteID, vlanConfig(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to create VLAN", err.Error())
		return
	}
	plan.ID, plan.SiteID = types.StringValue(id), types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vlanModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.client.FindVLAN(ctx, state.SiteID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read VLAN", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	applyVLAN(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// applyVLAN hydrates every modelled attribute from the controller, so a
// refresh detects drift and an imported VLAN lands in state complete rather
// than with null required attributes that would plan a replacement.
func applyVLAN(state *vlanModel, found *client.VLAN) {
	state.Name = types.StringValue(found.Name)
	state.VLANID = types.Int64Value(found.VLANID)
	state.DeviceMAC = types.StringValue(found.DeviceMAC)
	state.GatewaySubnet = types.StringValue(found.GatewaySubnet)
	state.Isolation = types.BoolValue(found.Isolation)
	state.DHCPEnabled = types.BoolValue(found.DHCPEnabled)
	state.DHCPLeaseTime = types.Int64Value(found.DHCPLeaseTime)
	state.DHCPDNSMode = types.StringValue(found.DHCPDNSMode)
	// Optional attributes the controller omits must stay null, or a config
	// that leaves them unset would diff against an empty string forever.
	state.DHCPRangeStart = optionalString(found.DHCPRangeStart)
	state.DHCPRangeEnd = optionalString(found.DHCPRangeEnd)
	state.DHCPPrimaryDNS = optionalString(found.DHCPPrimaryDNS)
}

func optionalString(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func (r *vlanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vlanModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.client.FindVLAN(ctx, state.SiteID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read VLAN before update", err.Error())
		return
	}
	if found == nil {
		resp.Diagnostics.AddError("Unable to update VLAN", "The VLAN no longer exists on the controller.")
		return
	}
	if err := r.client.UpdateVLAN(ctx, state.SiteID.ValueString(), found.ID, vlanConfig(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to update VLAN", err.Error())
		return
	}
	plan.ID, plan.SiteID = state.ID, state.SiteID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func vlanConfig(model vlanModel) client.VLANConfig {
	return client.VLANConfig{
		Name: model.Name.ValueString(), DeviceMAC: model.DeviceMAC.ValueString(), DeviceType: 1, VLANType: 0, VLANID: model.VLANID.ValueInt64(), GatewaySubnet: model.GatewaySubnet.ValueString(),
		DHCPEnabled: model.DHCPEnabled.ValueBool(), DHCPRangeStart: model.DHCPRangeStart.ValueString(), DHCPRangeEnd: model.DHCPRangeEnd.ValueString(), DHCPDNSMode: model.DHCPDNSMode.ValueString(), DHCPPrimaryDNS: model.DHCPPrimaryDNS.ValueString(), DHCPLeaseTime: model.DHCPLeaseTime.ValueInt64(), Isolation: model.Isolation.ValueBool(),
	}
}

func (r *vlanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vlanModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteVLAN(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete VLAN", err.Error())
	}
}

func (r *vlanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	siteNameOrID, id, ok := strings.Cut(req.ID, ":")
	if !ok || siteNameOrID == "" || id == "" {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected import ID in the form \"site_id:vlan_network_id\" (site_id may also be a site name), got: %q", req.ID))
		return
	}
	// Accept a site name as well as an ID, matching omada_dhcp_reservation:
	// newer Omada UIs don't always surface the site ID in the URL.
	siteID, err := r.client.ResolveSite(ctx, siteNameOrID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), siteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
