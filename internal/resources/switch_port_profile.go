package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var (
	_ resource.Resource                = &switchPortProfileResource{}
	_ resource.ResourceWithConfigure   = &switchPortProfileResource{}
	_ resource.ResourceWithImportState = &switchPortProfileResource{}
)

type switchPortProfileResource struct {
	client        *client.Client
	defaultSiteID string
}

// NewSwitchPortProfileResource returns the omada_switch_port_profile resource implementation.
func NewSwitchPortProfileResource() resource.Resource { return &switchPortProfileResource{} }

type switchPortProfileModel struct {
	ID                 types.String `tfsdk:"id"`
	SiteID             types.String `tfsdk:"site_id"`
	Name               types.String `tfsdk:"name"`
	NativeNetworkID    types.String `tfsdk:"native_network_id"`
	TaggedNetworkIDs   types.Set    `tfsdk:"tagged_network_ids"`
	UntaggedNetworkIDs types.Set    `tfsdk:"untagged_network_ids"`
	VLANConfigEnable   types.Bool   `tfsdk:"vlan_config_enable"`
	NetworkTagsSetting types.Int64  `tfsdk:"network_tags_setting"`
	POE                types.Int64  `tfsdk:"poe"`
	PortIsolation      types.Bool   `tfsdk:"port_isolation_enable"`
	LLDPMed            types.Bool   `tfsdk:"lldp_med_enable"`
	Dot1x              types.Int64  `tfsdk:"dot1x"`
	LoopbackDetect     types.Bool   `tfsdk:"loopback_detect_enable"`
	EEE                types.Bool   `tfsdk:"eee_enable"`
	FlowControl        types.Bool   `tfsdk:"flow_control_enable"`
	SpanningTree       types.Bool   `tfsdk:"spanning_tree_enable"`
	STPPriority        types.Int64  `tfsdk:"stp_priority"`
	STPExtPathCost     types.Int64  `tfsdk:"stp_ext_path_cost"`
	STPIntPathCost     types.Int64  `tfsdk:"stp_int_path_cost"`
	STPP2PLink         types.Int64  `tfsdk:"stp_p2p_link"`
	STPEdgePort        types.Bool   `tfsdk:"stp_edge_port"`
	STPLoopProtect     types.Bool   `tfsdk:"stp_loop_protect"`
	STPRootProtect     types.Bool   `tfsdk:"stp_root_protect"`
	STPTCGuard         types.Bool   `tfsdk:"stp_tc_guard"`
	STPBPDUProtect     types.Bool   `tfsdk:"stp_bpdu_protect"`
	STPBPDUFilter      types.Bool   `tfsdk:"stp_bpdu_filter"`
	STPBPDUForward     types.Bool   `tfsdk:"stp_bpdu_forward"`
}

func (r *switchPortProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_switch_port_profile"
}

func (r *switchPortProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A reusable Omada switch port profile. VLAN changes preserve unmodelled PoE, STP, LLDP and controller-owned settings.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site_id": schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"name":    schema.StringAttribute{Required: true, Description: "Profile display name."},
			"native_network_id": schema.StringAttribute{
				Optional: true, Computed: true, Description: "ID of the native/untagged VLAN network.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"tagged_network_ids": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType, Description: "VLAN network IDs carried tagged by this profile.",
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"untagged_network_ids": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType, Description: "Additional VLAN network IDs carried untagged by this profile.",
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"vlan_config_enable": schema.BoolAttribute{Optional: true, Computed: true, Description: "Enable the profile's VLAN configuration."},
			"network_tags_setting": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "VLAN tag policy: 0 Allow All, 1 Block All, or 2 Custom.",
				Validators:    []validator.Int64{int64validator.OneOf(0, 1, 2)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"poe":                    optionalComputedInt("PoE mode: 0 off, 1 on, or 2 keep the device setting."),
			"port_isolation_enable":  optionalComputedBool("Enable port isolation."),
			"lldp_med_enable":        optionalComputedBool("Enable LLDP-MED."),
			"dot1x":                  optionalComputedInt("802.1X controller mode."),
			"loopback_detect_enable": optionalComputedBool("Enable loopback detection."),
			"eee_enable":             optionalComputedBool("Enable Energy Efficient Ethernet."),
			"flow_control_enable":    optionalComputedBool("Enable Ethernet flow control."),
			"spanning_tree_enable":   optionalComputedBool("Enable spanning tree."),
			"stp_priority":           optionalComputedInt("STP port priority."),
			"stp_ext_path_cost":      optionalComputedInt("STP external path cost."),
			"stp_int_path_cost":      optionalComputedInt("STP internal path cost."),
			"stp_p2p_link":           optionalComputedInt("STP point-to-point controller mode."),
			"stp_edge_port":          optionalComputedBool("Treat the port as an STP edge port."),
			"stp_loop_protect":       optionalComputedBool("Enable STP loop protection."),
			"stp_root_protect":       optionalComputedBool("Enable STP root protection."),
			"stp_tc_guard":           optionalComputedBool("Enable STP topology-change guard."),
			"stp_bpdu_protect":       optionalComputedBool("Enable STP BPDU protection."),
			"stp_bpdu_filter":        optionalComputedBool("Enable STP BPDU filtering."),
			"stp_bpdu_forward":       optionalComputedBool("Enable STP BPDU forwarding."),
		},
	}
}

func optionalComputedBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{Optional: true, Computed: true, Description: description}
}

func optionalComputedInt(description string) schema.Int64Attribute {
	return schema.Int64Attribute{Optional: true, Computed: true, Description: description}
}

func (r *switchPortProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *switchPortProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan switchPortProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := resourceSiteID(plan.SiteID, r.defaultSiteID)
	id, err := r.client.CreateSwitchPortProfile(ctx, siteID, switchPortProfileConfig(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to create switch port profile", err.Error())
		return
	}
	plan.ID, plan.SiteID = types.StringValue(id), types.StringValue(siteID)
	found, err := r.client.FindSwitchPortProfile(ctx, siteID, id)
	if err != nil || found == nil {
		resp.Diagnostics.AddError("Unable to read switch port profile after creation", profileReadError(err, id))
		return
	}
	applySwitchPortProfile(&plan, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *switchPortProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state switchPortProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.client.FindSwitchPortProfile(ctx, state.SiteID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read switch port profile", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	applySwitchPortProfile(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *switchPortProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state switchPortProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateSwitchPortProfile(ctx, state.SiteID.ValueString(), state.ID.ValueString(), switchPortProfileConfig(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to update switch port profile", err.Error())
		return
	}
	plan.ID, plan.SiteID = state.ID, state.SiteID
	found, err := r.client.FindSwitchPortProfile(ctx, state.SiteID.ValueString(), state.ID.ValueString())
	if err != nil || found == nil {
		resp.Diagnostics.AddError("Unable to read switch port profile after update", profileReadError(err, state.ID.ValueString()))
		return
	}
	applySwitchPortProfile(&plan, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *switchPortProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state switchPortProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSwitchPortProfile(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete switch port profile", err.Error())
	}
}

func (r *switchPortProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	siteNameOrID, id, ok := strings.Cut(req.ID, ":")
	if !ok || siteNameOrID == "" || id == "" {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected import ID \"site_id:profile_id\", got: %q", req.ID))
		return
	}
	siteID, err := r.client.ResolveSite(ctx, siteNameOrID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), siteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func switchPortProfileConfig(model switchPortProfileModel) client.SwitchPortProfileConfig {
	cfg := client.SwitchPortProfileConfig{Name: model.Name.ValueString()}
	if !model.NativeNetworkID.IsNull() && !model.NativeNetworkID.IsUnknown() {
		value := model.NativeNetworkID.ValueString()
		cfg.NativeNetworkID = &value
	}
	if !model.TaggedNetworkIDs.IsNull() && !model.TaggedNetworkIDs.IsUnknown() {
		value := stringsFromSet(model.TaggedNetworkIDs)
		cfg.TaggedNetworkIDs = &value
	}
	if !model.UntaggedNetworkIDs.IsNull() && !model.UntaggedNetworkIDs.IsUnknown() {
		value := stringsFromSet(model.UntaggedNetworkIDs)
		cfg.UntaggedNetworkIDs = &value
	}
	if !model.VLANConfigEnable.IsNull() && !model.VLANConfigEnable.IsUnknown() {
		value := model.VLANConfigEnable.ValueBool()
		cfg.VLANConfigEnable = &value
	}
	if !model.NetworkTagsSetting.IsNull() && !model.NetworkTagsSetting.IsUnknown() {
		value := model.NetworkTagsSetting.ValueInt64()
		cfg.NetworkTagsSetting = &value
	}
	cfg.POE = intPointer(model.POE)
	cfg.PortIsolation = boolPointer(model.PortIsolation)
	cfg.LLDPMed = boolPointer(model.LLDPMed)
	cfg.Dot1x = intPointer(model.Dot1x)
	cfg.LoopbackDetect = boolPointer(model.LoopbackDetect)
	cfg.EEE = boolPointer(model.EEE)
	cfg.FlowControl = boolPointer(model.FlowControl)
	cfg.SpanningTree = boolPointer(model.SpanningTree)
	cfg.STPPriority = intPointer(model.STPPriority)
	cfg.STPExtPathCost = intPointer(model.STPExtPathCost)
	cfg.STPIntPathCost = intPointer(model.STPIntPathCost)
	cfg.STPP2PLink = intPointer(model.STPP2PLink)
	cfg.STPEdgePort = boolPointer(model.STPEdgePort)
	cfg.STPLoopProtect = boolPointer(model.STPLoopProtect)
	cfg.STPRootProtect = boolPointer(model.STPRootProtect)
	cfg.STPTCGuard = boolPointer(model.STPTCGuard)
	cfg.STPBPDUProtect = boolPointer(model.STPBPDUProtect)
	cfg.STPBPDUFilter = boolPointer(model.STPBPDUFilter)
	cfg.STPBPDUForward = boolPointer(model.STPBPDUForward)
	return cfg
}

func boolPointer(value types.Bool) *bool {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	result := value.ValueBool()
	return &result
}

func intPointer(value types.Int64) *int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	result := value.ValueInt64()
	return &result
}

func applySwitchPortProfile(state *switchPortProfileModel, found *client.SwitchPortProfile) {
	state.ID = types.StringValue(found.ID)
	state.Name = types.StringValue(found.Name)
	state.NativeNetworkID = types.StringValue(found.NativeNetworkID)
	state.TaggedNetworkIDs = stringSet(found.TaggedNetworkIDs)
	state.UntaggedNetworkIDs = stringSet(found.UntaggedNetworkIDs)
	state.VLANConfigEnable = types.BoolValue(found.VLANConfigEnable)
	state.NetworkTagsSetting = types.Int64Value(found.NetworkTagsSetting)
	state.POE = types.Int64Value(found.POE)
	state.PortIsolation = types.BoolValue(found.PortIsolation)
	state.LLDPMed = types.BoolValue(found.LLDPMed)
	state.Dot1x = types.Int64Value(found.Dot1x)
	state.LoopbackDetect = types.BoolValue(found.LoopbackDetect)
	state.EEE = types.BoolValue(found.EEE)
	state.FlowControl = types.BoolValue(found.FlowControl)
	state.SpanningTree = types.BoolValue(found.SpanningTree)
	state.STPPriority = types.Int64Value(found.STPPriority)
	state.STPExtPathCost = types.Int64Value(found.STPExtPathCost)
	state.STPIntPathCost = types.Int64Value(found.STPIntPathCost)
	state.STPP2PLink = types.Int64Value(found.STPP2PLink)
	state.STPEdgePort = types.BoolValue(found.STPEdgePort)
	state.STPLoopProtect = types.BoolValue(found.STPLoopProtect)
	state.STPRootProtect = types.BoolValue(found.STPRootProtect)
	state.STPTCGuard = types.BoolValue(found.STPTCGuard)
	state.STPBPDUProtect = types.BoolValue(found.STPBPDUProtect)
	state.STPBPDUFilter = types.BoolValue(found.STPBPDUFilter)
	state.STPBPDUForward = types.BoolValue(found.STPBPDUForward)
}

func profileReadError(err error, id string) string {
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("switch port profile %q was not returned by the controller", id)
}
