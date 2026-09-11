package datasources

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var _ datasource.DataSource = &switchDataSource{}
var _ datasource.DataSourceWithConfigure = &switchDataSource{}

type switchDataSource struct{ configuredDataSource }

func NewSwitchDataSource() datasource.DataSource { return &switchDataSource{} }

type switchPortDataModel struct {
	Port               types.Int64  `tfsdk:"port"`
	Name               types.String `tfsdk:"name"`
	ProfileID          types.String `tfsdk:"profile_id"`
	NativeNetworkID    types.String `tfsdk:"native_network_id"`
	NetworkTagsSetting types.Int64  `tfsdk:"network_tags_setting"`
	LinkSpeed          types.Int64  `tfsdk:"link_speed"`
	Duplex             types.Int64  `tfsdk:"duplex"`
}

type switchDataSourceModel struct {
	SiteID          types.String          `tfsdk:"site_id"`
	Name            types.String          `tfsdk:"name"`
	MAC             types.String          `tfsdk:"mac"`
	Model           types.String          `tfsdk:"model"`
	CompoundModel   types.String          `tfsdk:"compound_model"`
	SerialNumber    types.String          `tfsdk:"serial_number"`
	IP              types.String          `tfsdk:"ip"`
	Status          types.Int64           `tfsdk:"status"`
	StatusCategory  types.Int64           `tfsdk:"status_category"`
	FirmwareVersion types.String          `tfsdk:"firmware_version"`
	HardwareVersion types.String          `tfsdk:"hardware_version"`
	NeedUpgrade     types.Bool            `tfsdk:"need_upgrade"`
	UptimeSeconds   types.Int64           `tfsdk:"uptime_seconds"`
	ClientCount     types.Int64           `tfsdk:"client_count"`
	PortCount       types.Int64           `tfsdk:"port_count"`
	Ports           []switchPortDataModel `tfsdk:"ports"`
}

func (d *switchDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_switch"
}

func (d *switchDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up an adopted Omada switch by MAC address or name and returns its inventory, status and physical ports.",
		Attributes: map[string]schema.Attribute{
			"site_id":          schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site."},
			"name":             schema.StringAttribute{Optional: true, Computed: true, Description: "Switch name lookup selector and result."},
			"mac":              schema.StringAttribute{Optional: true, Computed: true, Description: "Switch MAC lookup selector and result."},
			"model":            schema.StringAttribute{Computed: true, Description: "Hardware model."},
			"compound_model":   schema.StringAttribute{Computed: true, Description: "Controller compound model identifier."},
			"serial_number":    schema.StringAttribute{Computed: true, Description: "Serial number."},
			"ip":               schema.StringAttribute{Computed: true, Description: "Management IP address."},
			"status":           schema.Int64Attribute{Computed: true, Description: "Raw controller status code."},
			"status_category":  schema.Int64Attribute{Computed: true, Description: "Status category: 0 disconnected, 1 connected, or 2 pending."},
			"firmware_version": schema.StringAttribute{Computed: true, Description: "Firmware version."},
			"hardware_version": schema.StringAttribute{Computed: true, Description: "Hardware version."},
			"need_upgrade":     schema.BoolAttribute{Computed: true, Description: "Whether a firmware update is available."},
			"uptime_seconds":   schema.Int64Attribute{Computed: true, Description: "Device uptime in seconds."},
			"client_count":     schema.Int64Attribute{Computed: true, Description: "Number of connected clients."},
			"port_count":       schema.Int64Attribute{Computed: true, Description: "Number of physical ports returned by the controller."},
			"ports": schema.ListNestedAttribute{
				Computed: true, Description: "Physical switch ports and their current profile/VLAN configuration.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"port":                 schema.Int64Attribute{Computed: true, Description: "Physical port number."},
					"name":                 schema.StringAttribute{Computed: true, Description: "Port display name."},
					"profile_id":           schema.StringAttribute{Computed: true, Description: "Assigned port-profile ID."},
					"native_network_id":    schema.StringAttribute{Computed: true, Description: "Native VLAN network ID."},
					"network_tags_setting": schema.Int64Attribute{Computed: true, Description: "VLAN tag policy."},
					"link_speed":           schema.Int64Attribute{Computed: true, Description: "Configured link-speed enum."},
					"duplex":               schema.Int64Attribute{Computed: true, Description: "Configured duplex enum."},
				}},
			},
		},
	}
}

func (d *switchDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req, resp)
}

func (d *switchDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config switchDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hasMAC, hasName := configuredString(config.MAC), configuredString(config.Name)
	if boolCount(hasMAC, hasName) != 1 {
		resp.Diagnostics.AddError("Invalid switch lookup", selectorError("mac or name"))
		return
	}
	siteID := selectedSiteID(config.SiteID, d.defaultSiteID)
	devices, err := d.client.ListDevices(ctx, siteID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Omada devices", err.Error())
		return
	}
	var matches []client.Device
	for _, device := range devices {
		if !strings.EqualFold(device.Type, "switch") {
			continue
		}
		if (hasMAC && client.NormalizeSwitchMAC(device.MAC) == client.NormalizeSwitchMAC(config.MAC.ValueString())) ||
			(hasName && strings.EqualFold(device.Name, config.Name.ValueString())) {
			matches = append(matches, device)
		}
	}
	if len(matches) != 1 {
		resp.Diagnostics.AddError("Unable to resolve Omada switch", fmt.Sprintf("The selector matched %d switches; exactly one is required.", len(matches)))
		return
	}
	device := matches[0]
	ports, err := d.client.ListSwitchPorts(ctx, siteID, device.MAC)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list switch ports", err.Error())
		return
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i].Port < ports[j].Port })
	state := switchDataSourceModel{
		SiteID: types.StringValue(siteID), Name: types.StringValue(device.Name), MAC: types.StringValue(device.MAC), Model: optionalString(device.Model), CompoundModel: optionalString(device.CompoundModel),
		SerialNumber: optionalString(device.SerialNumber), IP: optionalString(device.IP), Status: types.Int64Value(device.Status), StatusCategory: types.Int64Value(device.StatusCategory),
		FirmwareVersion: optionalString(device.FirmwareVersion), HardwareVersion: optionalString(device.HardwareVersion), NeedUpgrade: types.BoolValue(device.NeedUpgrade),
		UptimeSeconds: types.Int64Value(device.UptimeSeconds), ClientCount: types.Int64Value(device.ClientCount), PortCount: types.Int64Value(int64(len(ports))),
		Ports: make([]switchPortDataModel, 0, len(ports)),
	}
	for _, port := range ports {
		state.Ports = append(state.Ports, switchPortDataModel{
			Port: types.Int64Value(port.Port), Name: types.StringValue(port.Name), ProfileID: optionalString(port.ProfileID), NativeNetworkID: optionalString(port.NativeNetworkID),
			NetworkTagsSetting: types.Int64Value(port.NetworkTagsSetting), LinkSpeed: types.Int64Value(port.LinkSpeed), Duplex: types.Int64Value(port.Duplex),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
