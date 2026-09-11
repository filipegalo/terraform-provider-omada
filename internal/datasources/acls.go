package datasources

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var _ datasource.DataSource = &aclsDataSource{}
var _ datasource.DataSourceWithConfigure = &aclsDataSource{}

type aclsDataSource struct{ configuredDataSource }

// NewACLsDataSource returns the omada_acls data source implementation.
func NewACLsDataSource() datasource.DataSource { return &aclsDataSource{} }

type aclSummaryModel struct {
	ID      types.String `tfsdk:"id"`
	Type    types.String `tfsdk:"type"`
	Name    types.String `tfsdk:"name"`
	Enabled types.Bool   `tfsdk:"enabled"`
	Policy  types.String `tfsdk:"policy"`
}

type aclsDataSourceModel struct {
	SiteID types.String      `tfsdk:"site_id"`
	ACLs   []aclSummaryModel `tfsdk:"acls"`
}

func (d *aclsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acls"
}

func (d *aclsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists gateway, switch and EAP ACLs for discovery and import.",
		Attributes: map[string]schema.Attribute{
			"site_id": schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site."},
			"acls": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"id":      schema.StringAttribute{Computed: true},
				"type":    schema.StringAttribute{Computed: true},
				"name":    schema.StringAttribute{Computed: true},
				"enabled": schema.BoolAttribute{Computed: true},
				"policy":  schema.StringAttribute{Computed: true},
			}}},
		},
	}
}

func (d *aclsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req, resp)
}

func (d *aclsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config aclsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := selectedSiteID(config.SiteID, d.defaultSiteID)
	state := aclsDataSourceModel{SiteID: types.StringValue(siteID), ACLs: []aclSummaryModel{}}
	for _, item := range []struct {
		code int64
		name string
	}{{client.ACLTypeGateway, "gateway"}, {client.ACLTypeSwitch, "switch"}, {client.ACLTypeEAP, "eap"}} {
		rules, err := d.client.ListACLs(ctx, siteID, item.code)
		if err != nil {
			resp.Diagnostics.AddError("Unable to list "+item.name+" ACLs", err.Error())
			return
		}
		for _, rule := range rules {
			policy := "deny"
			if rule.Policy == 1 {
				policy = "permit"
			}
			state.ACLs = append(state.ACLs, aclSummaryModel{ID: types.StringValue(rule.ID), Type: types.StringValue(item.name), Name: types.StringValue(rule.Name), Enabled: types.BoolValue(rule.Enabled), Policy: types.StringValue(policy)})
		}
	}
	sort.Slice(state.ACLs, func(i, j int) bool {
		if state.ACLs[i].Type.ValueString() == state.ACLs[j].Type.ValueString() {
			return state.ACLs[i].Name.ValueString() < state.ACLs[j].Name.ValueString()
		}
		return state.ACLs[i].Type.ValueString() < state.ACLs[j].Type.ValueString()
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
