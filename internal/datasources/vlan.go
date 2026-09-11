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

var _ datasource.DataSource = &vlanDataSource{}
var _ datasource.DataSourceWithConfigure = &vlanDataSource{}

type vlanDataSource struct{ configuredDataSource }

// NewVLANDataSource returns the omada_vlan data source implementation.
func NewVLANDataSource() datasource.DataSource { return &vlanDataSource{} }

type vlanDataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	SiteID         types.String `tfsdk:"site_id"`
	Name           types.String `tfsdk:"name"`
	VLANID         types.Int64  `tfsdk:"vlan_id"`
	Primary        types.Bool   `tfsdk:"primary"`
	DeviceMAC      types.String `tfsdk:"device_mac"`
	GatewaySubnet  types.String `tfsdk:"gateway_subnet"`
	Isolation      types.Bool   `tfsdk:"isolation"`
	DHCPEnabled    types.Bool   `tfsdk:"dhcp_enabled"`
	DHCPRangeStart types.String `tfsdk:"dhcp_range_start"`
	DHCPRangeEnd   types.String `tfsdk:"dhcp_range_end"`
}

func (d *vlanDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vlan"
}

func (d *vlanDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up an existing Omada VLAN by ID, name, or IEEE 802.1Q VLAN ID.",
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Optional: true, Computed: true, Description: "Omada network ID lookup selector and result."},
			"site_id":          schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site."},
			"name":             schema.StringAttribute{Optional: true, Computed: true, Description: "VLAN name lookup selector and result."},
			"vlan_id":          schema.Int64Attribute{Optional: true, Computed: true, Description: "802.1Q VLAN ID lookup selector and result."},
			"primary":          schema.BoolAttribute{Computed: true, Description: "Whether this is the site's primary LAN."},
			"device_mac":       schema.StringAttribute{Computed: true, Description: "MAC address of the gateway owning the VLAN."},
			"gateway_subnet":   schema.StringAttribute{Computed: true, Description: "Gateway address and CIDR."},
			"isolation":        schema.BoolAttribute{Computed: true, Description: "Whether inter-VLAN isolation is enabled."},
			"dhcp_enabled":     schema.BoolAttribute{Computed: true, Description: "Whether the gateway DHCP server is enabled."},
			"dhcp_range_start": schema.StringAttribute{Computed: true, Description: "First DHCP pool address, when present."},
			"dhcp_range_end":   schema.StringAttribute{Computed: true, Description: "Last DHCP pool address, when present."},
		},
	}
}

func (d *vlanDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req, resp)
}

func (d *vlanDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config vlanDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hasID := configuredString(config.ID)
	hasName := configuredString(config.Name)
	hasVLANID := !config.VLANID.IsNull() && !config.VLANID.IsUnknown()
	if boolCount(hasID, hasName, hasVLANID) != 1 {
		resp.Diagnostics.AddError("Invalid VLAN lookup", selectorError("id, name, or vlan_id"))
		return
	}
	siteID := selectedSiteID(config.SiteID, d.defaultSiteID)
	vlans, err := d.client.ListVLANs(ctx, siteID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Omada VLANs", err.Error())
		return
	}
	var matches []client.VLAN
	for _, vlan := range vlans {
		if (hasID && vlan.ID == config.ID.ValueString()) ||
			(hasName && strings.EqualFold(vlan.Name, config.Name.ValueString())) ||
			(hasVLANID && vlan.VLANID == config.VLANID.ValueInt64()) {
			matches = append(matches, vlan)
		}
	}
	if len(matches) != 1 {
		resp.Diagnostics.AddError("Unable to resolve Omada VLAN", fmt.Sprintf("The selector matched %d VLANs; exactly one is required.", len(matches)))
		return
	}
	vlan := matches[0]
	state := vlanDataSourceModel{
		ID: types.StringValue(vlan.ID), SiteID: types.StringValue(siteID), Name: types.StringValue(vlan.Name), VLANID: types.Int64Value(vlan.VLANID), Primary: types.BoolValue(vlan.Primary),
		DeviceMAC: types.StringValue(vlan.DeviceMAC), GatewaySubnet: optionalString(vlan.GatewaySubnet), Isolation: types.BoolValue(vlan.Isolation), DHCPEnabled: types.BoolValue(vlan.DHCPEnabled),
		DHCPRangeStart: optionalString(vlan.DHCPRangeStart), DHCPRangeEnd: optionalString(vlan.DHCPRangeEnd),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
