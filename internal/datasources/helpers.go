package datasources

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

type configuredDataSource struct {
	client        *client.Client
	defaultSiteID string
}

func (d *configuredDataSource) configure(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*client.ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *client.ProviderData, got: %T", req.ProviderData))
		return
	}
	d.client, d.defaultSiteID = data.Client, data.SiteID
}

func selectorError(kind string) string {
	return fmt.Sprintf("Configure exactly one lookup selector for %s.", kind)
}
