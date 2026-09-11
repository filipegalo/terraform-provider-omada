package datasources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var _ datasource.DataSource = &switchPortProfileDataSource{}
var _ datasource.DataSourceWithConfigure = &switchPortProfileDataSource{}

type switchPortProfileDataSource struct{ configuredDataSource }

// NewSwitchPortProfileDataSource returns the omada_switch_port_profile data source implementation.
func NewSwitchPortProfileDataSource() datasource.DataSource { return &switchPortProfileDataSource{} }

type switchPortProfileDataSourceModel struct {
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

func (d *switchPortProfileDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_switch_port_profile"
}

func (d *switchPortProfileDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up an existing Omada switch port profile by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id":                     schema.StringAttribute{Optional: true, Computed: true, Description: "Profile ID lookup selector and result."},
			"site_id":                schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site."},
			"name":                   schema.StringAttribute{Optional: true, Computed: true, Description: "Profile name lookup selector and result."},
			"native_network_id":      schema.StringAttribute{Computed: true, Description: "Native/untagged VLAN network ID."},
			"tagged_network_ids":     schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "Tagged VLAN network IDs."},
			"untagged_network_ids":   schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "Additional untagged VLAN network IDs."},
			"vlan_config_enable":     schema.BoolAttribute{Computed: true, Description: "Whether profile VLAN configuration is enabled."},
			"network_tags_setting":   schema.Int64Attribute{Computed: true, Description: "VLAN tag policy: 0 Allow All, 1 Block All, or 2 Custom."},
			"poe":                    schema.Int64Attribute{Computed: true, Description: "PoE mode."},
			"port_isolation_enable":  schema.BoolAttribute{Computed: true, Description: "Whether port isolation is enabled."},
			"lldp_med_enable":        schema.BoolAttribute{Computed: true, Description: "Whether LLDP-MED is enabled."},
			"dot1x":                  schema.Int64Attribute{Computed: true, Description: "802.1X controller mode."},
			"loopback_detect_enable": schema.BoolAttribute{Computed: true, Description: "Whether loopback detection is enabled."},
			"eee_enable":             schema.BoolAttribute{Computed: true, Description: "Whether Energy Efficient Ethernet is enabled."},
			"flow_control_enable":    schema.BoolAttribute{Computed: true, Description: "Whether flow control is enabled."},
			"spanning_tree_enable":   schema.BoolAttribute{Computed: true, Description: "Whether spanning tree is enabled."},
			"stp_priority":           schema.Int64Attribute{Computed: true, Description: "STP port priority."},
			"stp_ext_path_cost":      schema.Int64Attribute{Computed: true, Description: "STP external path cost."},
			"stp_int_path_cost":      schema.Int64Attribute{Computed: true, Description: "STP internal path cost."},
			"stp_p2p_link":           schema.Int64Attribute{Computed: true, Description: "STP point-to-point controller mode."},
			"stp_edge_port":          schema.BoolAttribute{Computed: true, Description: "Whether this is an STP edge port."},
			"stp_loop_protect":       schema.BoolAttribute{Computed: true, Description: "Whether STP loop protection is enabled."},
			"stp_root_protect":       schema.BoolAttribute{Computed: true, Description: "Whether STP root protection is enabled."},
			"stp_tc_guard":           schema.BoolAttribute{Computed: true, Description: "Whether STP topology-change guard is enabled."},
			"stp_bpdu_protect":       schema.BoolAttribute{Computed: true, Description: "Whether STP BPDU protection is enabled."},
			"stp_bpdu_filter":        schema.BoolAttribute{Computed: true, Description: "Whether STP BPDU filtering is enabled."},
			"stp_bpdu_forward":       schema.BoolAttribute{Computed: true, Description: "Whether STP BPDU forwarding is enabled."},
		},
	}
}

func (d *switchPortProfileDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req, resp)
}

func (d *switchPortProfileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config switchPortProfileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hasID, hasName := configuredString(config.ID), configuredString(config.Name)
	if boolCount(hasID, hasName) != 1 {
		resp.Diagnostics.AddError("Invalid switch port profile lookup", selectorError("id or name"))
		return
	}
	siteID := selectedSiteID(config.SiteID, d.defaultSiteID)
	profiles, err := d.client.ListSwitchPortProfiles(ctx, siteID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list switch port profiles", err.Error())
		return
	}
	var matches []client.SwitchPortProfile
	for _, profile := range profiles {
		if (hasID && profile.ID == config.ID.ValueString()) || (hasName && strings.EqualFold(profile.Name, config.Name.ValueString())) {
			matches = append(matches, profile)
		}
	}
	if len(matches) != 1 {
		resp.Diagnostics.AddError("Unable to resolve switch port profile", fmt.Sprintf("The selector matched %d profiles; exactly one is required.", len(matches)))
		return
	}
	profile := matches[0]
	state := switchPortProfileDataSourceModel{
		ID: types.StringValue(profile.ID), SiteID: types.StringValue(siteID), Name: types.StringValue(profile.Name), NativeNetworkID: optionalString(profile.NativeNetworkID),
		TaggedNetworkIDs: stringSet(profile.TaggedNetworkIDs), UntaggedNetworkIDs: stringSet(profile.UntaggedNetworkIDs), VLANConfigEnable: types.BoolValue(profile.VLANConfigEnable), NetworkTagsSetting: types.Int64Value(profile.NetworkTagsSetting),
		POE: types.Int64Value(profile.POE), PortIsolation: types.BoolValue(profile.PortIsolation), LLDPMed: types.BoolValue(profile.LLDPMed), Dot1x: types.Int64Value(profile.Dot1x),
		LoopbackDetect: types.BoolValue(profile.LoopbackDetect), EEE: types.BoolValue(profile.EEE), FlowControl: types.BoolValue(profile.FlowControl), SpanningTree: types.BoolValue(profile.SpanningTree),
		STPPriority: types.Int64Value(profile.STPPriority), STPExtPathCost: types.Int64Value(profile.STPExtPathCost), STPIntPathCost: types.Int64Value(profile.STPIntPathCost), STPP2PLink: types.Int64Value(profile.STPP2PLink),
		STPEdgePort: types.BoolValue(profile.STPEdgePort), STPLoopProtect: types.BoolValue(profile.STPLoopProtect), STPRootProtect: types.BoolValue(profile.STPRootProtect), STPTCGuard: types.BoolValue(profile.STPTCGuard),
		STPBPDUProtect: types.BoolValue(profile.STPBPDUProtect), STPBPDUFilter: types.BoolValue(profile.STPBPDUFilter), STPBPDUForward: types.BoolValue(profile.STPBPDUForward),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
