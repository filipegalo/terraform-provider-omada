package datasources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &siteDataSource{}
var _ datasource.DataSourceWithConfigure = &siteDataSource{}

type siteDataSource struct{ configuredDataSource }

func NewSiteDataSource() datasource.DataSource { return &siteDataSource{} }

type siteDataSourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (d *siteDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site"
}

func (d *siteDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up an Omada site by name or ID. With no selector, returns the provider's configured site.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID lookup selector and result."},
			"name": schema.StringAttribute{Optional: true, Computed: true, Description: "Site name lookup selector and result."},
		},
	}
}

func (d *siteDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req, resp)
}

func (d *siteDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config siteDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hasID := !config.ID.IsNull() && !config.ID.IsUnknown() && config.ID.ValueString() != ""
	hasName := !config.Name.IsNull() && !config.Name.IsUnknown() && config.Name.ValueString() != ""
	if hasID && hasName {
		resp.Diagnostics.AddError("Ambiguous site lookup", "Configure only one of id or name.")
		return
	}
	sites, err := d.client.ListSites(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Omada sites", err.Error())
		return
	}
	targetID := d.defaultSiteID
	if hasID {
		targetID = config.ID.ValueString()
	}
	for _, site := range sites {
		matches := site.ID == targetID
		if hasName {
			matches = strings.EqualFold(site.Name, config.Name.ValueString())
		}
		if matches {
			state := siteDataSourceModel{ID: types.StringValue(site.ID), Name: types.StringValue(site.Name)}
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	selector := targetID
	if hasName {
		selector = config.Name.ValueString()
	}
	resp.Diagnostics.AddError("Omada site not found", fmt.Sprintf("No site matched %q.", selector))
}
