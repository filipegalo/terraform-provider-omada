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

var _ datasource.DataSource = &wlanGroupDataSource{}
var _ datasource.DataSourceWithConfigure = &wlanGroupDataSource{}

type wlanGroupDataSource struct{ configuredDataSource }

// NewWLANGroupDataSource returns the omada_wlan_group data source implementation.
func NewWLANGroupDataSource() datasource.DataSource { return &wlanGroupDataSource{} }

type wlanGroupDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	SiteID  types.String `tfsdk:"site_id"`
	Name    types.String `tfsdk:"name"`
	Primary types.Bool   `tfsdk:"primary"`
}

func (d *wlanGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wlan_group"
}

func (d *wlanGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up an Omada WLAN group by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Optional: true, Computed: true, Description: "WLAN group ID lookup selector and result."},
			"site_id": schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site."},
			"name":    schema.StringAttribute{Optional: true, Computed: true, Description: "WLAN group name lookup selector and result."},
			"primary": schema.BoolAttribute{Computed: true, Description: "Whether this is the primary WLAN group."},
		},
	}
}

func (d *wlanGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req, resp)
}

func (d *wlanGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config wlanGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hasID, hasName := configuredString(config.ID), configuredString(config.Name)
	if boolCount(hasID, hasName) != 1 {
		resp.Diagnostics.AddError("Invalid WLAN group lookup", selectorError("id or name"))
		return
	}
	siteID := selectedSiteID(config.SiteID, d.defaultSiteID)
	groups, err := d.client.ListWLANGroups(ctx, siteID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list WLAN groups", err.Error())
		return
	}
	var matches []client.WLANGroup
	for _, group := range groups {
		if (hasID && group.ID == config.ID.ValueString()) || (hasName && strings.EqualFold(group.Name, config.Name.ValueString())) {
			matches = append(matches, group)
		}
	}
	if len(matches) != 1 {
		resp.Diagnostics.AddError("Unable to resolve WLAN group", fmt.Sprintf("The selector matched %d WLAN groups; exactly one is required.", len(matches)))
		return
	}
	group := matches[0]
	state := wlanGroupDataSourceModel{ID: types.StringValue(group.ID), SiteID: types.StringValue(siteID), Name: types.StringValue(group.Name), Primary: types.BoolValue(group.Primary)}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
