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
}

func (d *switchPortProfileDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_switch_port_profile"
}

func (d *switchPortProfileDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up an existing Omada switch port profile by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id":                   schema.StringAttribute{Optional: true, Computed: true, Description: "Profile ID lookup selector and result."},
			"site_id":              schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site."},
			"name":                 schema.StringAttribute{Optional: true, Computed: true, Description: "Profile name lookup selector and result."},
			"native_network_id":    schema.StringAttribute{Computed: true, Description: "Native/untagged VLAN network ID."},
			"tagged_network_ids":   schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "Tagged VLAN network IDs."},
			"untagged_network_ids": schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "Additional untagged VLAN network IDs."},
			"vlan_config_enable":   schema.BoolAttribute{Computed: true, Description: "Whether profile VLAN configuration is enabled."},
			"network_tags_setting": schema.Int64Attribute{Computed: true, Description: "VLAN tag policy: 0 Allow All, 1 Block All, or 2 Custom."},
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
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
