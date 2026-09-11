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

var _ datasource.DataSource = &ssidDataSource{}
var _ datasource.DataSourceWithConfigure = &ssidDataSource{}

type ssidDataSource struct{ configuredDataSource }

// NewSSIDDataSource returns the omada_ssid data source implementation.
func NewSSIDDataSource() datasource.DataSource { return &ssidDataSource{} }

type ssidDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	SiteID      types.String `tfsdk:"site_id"`
	WLANGroupID types.String `tfsdk:"wlan_group_id"`
	Name        types.String `tfsdk:"name"`
	Band        types.Int64  `tfsdk:"band"`
	Security    types.Int64  `tfsdk:"security"`
	Broadcast   types.Bool   `tfsdk:"broadcast"`
	VLANEnable  types.Bool   `tfsdk:"vlan_enable"`
	VLANID      types.Int64  `tfsdk:"vlan_id"`
	Guest       types.Bool   `tfsdk:"guest"`
	Enable11r   types.Bool   `tfsdk:"enable_11r"`
	PMFMode     types.Int64  `tfsdk:"pmf_mode"`
}

func (d *ssidDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssid"
}

func (d *ssidDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up one SSID by ID or name within a WLAN group.",
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Optional: true, Computed: true, Description: "SSID controller ID. Specify either id or name."},
			"site_id":       schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site."},
			"wlan_group_id": schema.StringAttribute{Required: true, Description: "WLAN group ID."},
			"name":          schema.StringAttribute{Optional: true, Computed: true, Description: "SSID name. Specify either id or name."},
			"band":          schema.Int64Attribute{Computed: true},
			"security":      schema.Int64Attribute{Computed: true},
			"broadcast":     schema.BoolAttribute{Computed: true},
			"vlan_enable":   schema.BoolAttribute{Computed: true},
			"vlan_id":       schema.Int64Attribute{Computed: true},
			"guest":         schema.BoolAttribute{Computed: true},
			"enable_11r":    schema.BoolAttribute{Computed: true},
			"pmf_mode":      schema.Int64Attribute{Computed: true},
		},
	}
}

func (d *ssidDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req, resp)
}

func (d *ssidDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ssidDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.ID.ValueString() == "" && config.Name.ValueString() == "" {
		resp.Diagnostics.AddError("Missing SSID selector", "Set either id or name.")
		return
	}
	ssids, err := d.client.ListSSIDs(ctx, selectedSiteID(config.SiteID, d.defaultSiteID), config.WLANGroupID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list SSIDs", err.Error())
		return
	}
	var found *client.SSID
	for i := range ssids {
		idMatch := config.ID.ValueString() != "" && ssids[i].ID == config.ID.ValueString()
		nameMatch := config.Name.ValueString() != "" && strings.EqualFold(ssids[i].Name, config.Name.ValueString())
		if idMatch || nameMatch {
			if found != nil {
				resp.Diagnostics.AddError("Ambiguous SSID", fmt.Sprintf("More than one SSID matches %q.", config.Name.ValueString()))
				return
			}
			found = &ssids[i]
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("SSID not found", "No SSID matched the configured id or name.")
		return
	}
	config.SiteID = types.StringValue(selectedSiteID(config.SiteID, d.defaultSiteID))
	config.ID, config.Name = types.StringValue(found.ID), types.StringValue(found.Name)
	config.Band, config.Security = types.Int64Value(found.Band), types.Int64Value(found.Security)
	config.Broadcast, config.VLANEnable = types.BoolValue(found.Broadcast), types.BoolValue(found.VLANEnable)
	config.VLANID, config.Guest = types.Int64Value(found.VLANID), types.BoolValue(found.Guest)
	config.Enable11r, config.PMFMode = types.BoolValue(found.Enable11r), types.Int64Value(found.PMFMode)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
