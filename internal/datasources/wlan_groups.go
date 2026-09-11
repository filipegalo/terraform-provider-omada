package datasources

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &wlanGroupsDataSource{}
var _ datasource.DataSourceWithConfigure = &wlanGroupsDataSource{}

type wlanGroupsDataSource struct{ configuredDataSource }

// NewWLANGroupsDataSource returns the omada_wlan_groups data source implementation.
func NewWLANGroupsDataSource() datasource.DataSource { return &wlanGroupsDataSource{} }

type wlanGroupSummaryModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Primary types.Bool   `tfsdk:"primary"`
}

type wlanGroupsDataSourceModel struct {
	SiteID types.String            `tfsdk:"site_id"`
	Groups []wlanGroupSummaryModel `tfsdk:"groups"`
}

func (d *wlanGroupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wlan_groups"
}

func (d *wlanGroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists WLAN groups for discovery and import.",
		Attributes: map[string]schema.Attribute{
			"site_id": schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site."},
			"groups": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"id":      schema.StringAttribute{Computed: true},
				"name":    schema.StringAttribute{Computed: true},
				"primary": schema.BoolAttribute{Computed: true},
			}}},
		},
	}
}

func (d *wlanGroupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req, resp)
}

func (d *wlanGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config wlanGroupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := selectedSiteID(config.SiteID, d.defaultSiteID)
	groups, err := d.client.ListWLANGroups(ctx, siteID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list WLAN groups", err.Error())
		return
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	state := wlanGroupsDataSourceModel{SiteID: types.StringValue(siteID), Groups: make([]wlanGroupSummaryModel, 0, len(groups))}
	for _, group := range groups {
		state.Groups = append(state.Groups, wlanGroupSummaryModel{ID: types.StringValue(group.ID), Name: types.StringValue(group.Name), Primary: types.BoolValue(group.Primary)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
