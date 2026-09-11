package datasources

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ssidsDataSource{}
var _ datasource.DataSourceWithConfigure = &ssidsDataSource{}

type ssidsDataSource struct{ configuredDataSource }

// NewSSIDsDataSource returns the omada_ssids data source implementation.
func NewSSIDsDataSource() datasource.DataSource { return &ssidsDataSource{} }

type ssidSummaryModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Band       types.Int64  `tfsdk:"band"`
	Security   types.Int64  `tfsdk:"security"`
	Broadcast  types.Bool   `tfsdk:"broadcast"`
	VLANEnable types.Bool   `tfsdk:"vlan_enable"`
	VLANID     types.Int64  `tfsdk:"vlan_id"`
	Guest      types.Bool   `tfsdk:"guest"`
}

type ssidsDataSourceModel struct {
	SiteID      types.String       `tfsdk:"site_id"`
	WLANGroupID types.String       `tfsdk:"wlan_group_id"`
	SSIDs       []ssidSummaryModel `tfsdk:"ssids"`
}

func (d *ssidsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssids"
}

func (d *ssidsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists SSIDs in one WLAN group for discovery and import.",
		Attributes: map[string]schema.Attribute{
			"site_id":       schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site."},
			"wlan_group_id": schema.StringAttribute{Required: true, Description: "WLAN group ID."},
			"ssids": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"id":          schema.StringAttribute{Computed: true},
				"name":        schema.StringAttribute{Computed: true},
				"band":        schema.Int64Attribute{Computed: true},
				"security":    schema.Int64Attribute{Computed: true},
				"broadcast":   schema.BoolAttribute{Computed: true},
				"vlan_enable": schema.BoolAttribute{Computed: true},
				"vlan_id":     schema.Int64Attribute{Computed: true},
				"guest":       schema.BoolAttribute{Computed: true},
			}}},
		},
	}
}

func (d *ssidsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req, resp)
}

func (d *ssidsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ssidsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := selectedSiteID(config.SiteID, d.defaultSiteID)
	ssids, err := d.client.ListSSIDs(ctx, siteID, config.WLANGroupID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list SSIDs", err.Error())
		return
	}
	sort.Slice(ssids, func(i, j int) bool { return ssids[i].Name < ssids[j].Name })
	state := ssidsDataSourceModel{SiteID: types.StringValue(siteID), WLANGroupID: config.WLANGroupID, SSIDs: make([]ssidSummaryModel, 0, len(ssids))}
	for _, ssid := range ssids {
		state.SSIDs = append(state.SSIDs, ssidSummaryModel{
			ID: types.StringValue(ssid.ID), Name: types.StringValue(ssid.Name), Band: types.Int64Value(ssid.Band), Security: types.Int64Value(ssid.Security),
			Broadcast: types.BoolValue(ssid.Broadcast), VLANEnable: types.BoolValue(ssid.VLANEnable), VLANID: types.Int64Value(ssid.VLANID), Guest: types.BoolValue(ssid.Guest),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
