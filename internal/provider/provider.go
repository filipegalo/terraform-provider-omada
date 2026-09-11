// Package provider implements the omada Terraform provider: schema and
// Configure. Resource registration lives here too, since
// terraform-plugin-framework requires the provider to list its resource
// constructors.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
	"github.com/filipegalo/terraform-provider-omada/internal/datasources"
	"github.com/filipegalo/terraform-provider-omada/internal/resources"
)

var (
	_ provider.Provider = &omadaProvider{}
)

type omadaProvider struct {
	version string
}

// New returns a provider.Provider factory, as required by
// providerserver.NewProtocol6(...). version should be set to the release
// version at build time (or "dev" for local builds).
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &omadaProvider{version: version}
	}
}

type providerModel struct {
	BaseURL       types.String `tfsdk:"base_url"`
	Username      types.String `tfsdk:"username"`
	Password      types.String `tfsdk:"password"`
	ClientID      types.String `tfsdk:"client_id"`
	ClientSecret  types.String `tfsdk:"client_secret"`
	Site          types.String `tfsdk:"site"`
	SkipTLSVerify types.Bool   `tfsdk:"skip_tls_verify"`
}

func (p *omadaProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "omada"
	resp.Version = p.version
}

func (p *omadaProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a TP-Link Omada Controller (local, non-cloud API).",
		Attributes: map[string]schema.Attribute{
			"base_url": schema.StringAttribute{
				Optional:    true,
				Description: "Base URL of the Omada Controller, e.g. https://192.168.1.1:8043. Falls back to the OMADA_URL environment variable.",
			},
			"username": schema.StringAttribute{
				Optional:    true,
				Description: "Local Omada admin username. Falls back to the OMADA_USERNAME environment variable.",
			},
			"password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Local Omada admin password. Falls back to the OMADA_PASSWORD environment variable.",
			},
			"client_id": schema.StringAttribute{
				Optional:    true,
				Description: "Public Open API application Client ID. Falls back to the OMADA_CLIENT_ID environment variable. Must be set together with client_secret.",
			},
			"client_secret": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Public Open API application Client Secret. Falls back to the OMADA_CLIENT_SECRET environment variable. Must be set together with client_id.",
			},
			"site": schema.StringAttribute{
				Optional:    true,
				Description: "Site name or ID to resolve at startup. Falls back to the OMADA_SITE environment variable.",
			},
			"skip_tls_verify": schema.BoolAttribute{
				Optional:    true,
				Description: "Skip TLS certificate verification, useful for controllers with self-signed certificates. Falls back to the OMADA_SKIP_TLS_VERIFY environment variable (\"true\"/\"false\").",
			},
		},
	}
}

func (p *omadaProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	baseURL := stringOrEnv(config.BaseURL, "OMADA_URL")
	username := stringOrEnv(config.Username, "OMADA_USERNAME")
	password := stringOrEnv(config.Password, "OMADA_PASSWORD")
	clientID := stringOrEnv(config.ClientID, "OMADA_CLIENT_ID")
	clientSecret := stringOrEnv(config.ClientSecret, "OMADA_CLIENT_SECRET")
	site := stringOrEnv(config.Site, "OMADA_SITE")
	skipTLSVerify := boolOrEnv(config.SkipTLSVerify, "OMADA_SKIP_TLS_VERIFY")

	if baseURL == "" {
		resp.Diagnostics.AddAttributeError(path.Root("base_url"), "Missing base_url", "Set base_url in the provider configuration or the OMADA_URL environment variable.")
	}
	if username == "" {
		resp.Diagnostics.AddAttributeError(path.Root("username"), "Missing username", "Set username in the provider configuration or the OMADA_USERNAME environment variable.")
	}
	if password == "" {
		resp.Diagnostics.AddAttributeError(path.Root("password"), "Missing password", "Set password in the provider configuration or the OMADA_PASSWORD environment variable.")
	}
	if (clientID == "") != (clientSecret == "") {
		resp.Diagnostics.AddError("Incomplete Open API credentials", "Set both client_id and client_secret, or omit both to use classic authentication for every request.")
	}
	if site == "" {
		resp.Diagnostics.AddAttributeError(path.Root("site"), "Missing site", "Set site in the provider configuration or the OMADA_SITE environment variable.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	c, err := client.NewClient(baseURL, username, password, clientID, clientSecret, skipTLSVerify)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create Omada client", err.Error())
		return
	}

	if err := c.Authenticate(ctx); err != nil {
		resp.Diagnostics.AddError("Unable to authenticate with Omada Controller", err.Error())
		return
	}

	siteID, err := c.ResolveSite(ctx, site)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("site"), "Unable to resolve site", err.Error())
		return
	}

	data := &client.ProviderData{Client: c, SiteID: siteID}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *omadaProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewDHCPReservationResource,
		resources.NewVLANResource,
		resources.NewSwitchPortResource,
		resources.NewSwitchPortProfileResource,
		resources.NewWLANGroupResource,
		resources.NewSSIDResource,
		resources.NewACLResource,
	}
}

func (p *omadaProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasources.NewSiteDataSource,
		datasources.NewVLANDataSource,
		datasources.NewSwitchPortProfileDataSource,
		datasources.NewSwitchDataSource,
		datasources.NewWLANGroupDataSource,
		datasources.NewWLANGroupsDataSource,
		datasources.NewSSIDDataSource,
		datasources.NewSSIDsDataSource,
		datasources.NewACLDataSource,
		datasources.NewACLsDataSource,
	}
}

func stringOrEnv(v types.String, envKey string) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	return os.Getenv(envKey)
}

func boolOrEnv(v types.Bool, envKey string) bool {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueBool()
	}
	return os.Getenv(envKey) == "true"
}
