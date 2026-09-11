package datasources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var _ datasource.DataSource = &aclDataSource{}
var _ datasource.DataSourceWithConfigure = &aclDataSource{}

type aclDataSource struct{ configuredDataSource }

// NewACLDataSource returns the omada_acl data source implementation.
func NewACLDataSource() datasource.DataSource { return &aclDataSource{} }

type aclDataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	SiteID          types.String `tfsdk:"site_id"`
	Type            types.String `tfsdk:"type"`
	Name            types.String `tfsdk:"name"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	Policy          types.String `tfsdk:"policy"`
	Protocols       types.Set    `tfsdk:"protocols"`
	SourceType      types.Int64  `tfsdk:"source_type"`
	SourceIDs       types.Set    `tfsdk:"source_ids"`
	DestinationType types.Int64  `tfsdk:"destination_type"`
	DestinationIDs  types.Set    `tfsdk:"destination_ids"`
	LANToWAN        types.Bool   `tfsdk:"lan_to_wan"`
	LANToLAN        types.Bool   `tfsdk:"lan_to_lan"`
	WANInIDs        types.Set    `tfsdk:"wan_in_ids"`
	VPNInIDs        types.Set    `tfsdk:"vpn_in_ids"`
}

var aclDataSourceTypes = map[string]int64{"gateway": client.ACLTypeGateway, "switch": client.ACLTypeSwitch, "eap": client.ACLTypeEAP}

func (d *aclDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acl"
}

func (d *aclDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Looks up one gateway, switch or EAP ACL by ID or name.", Attributes: map[string]schema.Attribute{
		"id":      schema.StringAttribute{Optional: true, Computed: true, Description: "ACL controller ID. Specify either id or name."},
		"site_id": schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site."},
		"type":    schema.StringAttribute{Optional: true, Computed: true, Description: "ACL surface. Defaults to gateway.", Validators: []validator.String{stringvalidator.OneOf("gateway", "switch", "eap")}},
		"name":    schema.StringAttribute{Optional: true, Computed: true, Description: "ACL name. Specify either id or name."},
		"enabled": schema.BoolAttribute{Computed: true}, "policy": schema.StringAttribute{Computed: true},
		"protocols":   schema.SetAttribute{Computed: true, ElementType: types.Int64Type},
		"source_type": schema.Int64Attribute{Computed: true}, "source_ids": schema.SetAttribute{Computed: true, ElementType: types.StringType},
		"destination_type": schema.Int64Attribute{Computed: true}, "destination_ids": schema.SetAttribute{Computed: true, ElementType: types.StringType},
		"lan_to_wan": schema.BoolAttribute{Computed: true}, "lan_to_lan": schema.BoolAttribute{Computed: true},
		"wan_in_ids": schema.SetAttribute{Computed: true, ElementType: types.StringType}, "vpn_in_ids": schema.SetAttribute{Computed: true, ElementType: types.StringType},
	}}
}

func (d *aclDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req, resp)
}

func (d *aclDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config aclDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.ID.ValueString() == "" && config.Name.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ACL selector", "Set either id or name.")
		return
	}
	siteID := selectedSiteID(config.SiteID, d.defaultSiteID)
	typeName := config.Type.ValueString()
	if typeName == "" {
		typeName = "gateway"
	}
	rules, err := d.client.ListACLs(ctx, siteID, aclDataSourceTypes[typeName])
	if err != nil {
		resp.Diagnostics.AddError("Unable to list ACLs", err.Error())
		return
	}
	var found *client.ACL
	for i := range rules {
		if (config.ID.ValueString() != "" && rules[i].ID == config.ID.ValueString()) || (config.Name.ValueString() != "" && strings.EqualFold(rules[i].Name, config.Name.ValueString())) {
			if found != nil {
				resp.Diagnostics.AddError("Ambiguous ACL", fmt.Sprintf("More than one ACL matches %q.", config.Name.ValueString()))
				return
			}
			found = &rules[i]
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("ACL not found", "No ACL matched the configured id or name.")
		return
	}
	config.SiteID, config.ID, config.Name, config.Type = types.StringValue(siteID), types.StringValue(found.ID), types.StringValue(found.Name), types.StringValue(typeName)
	config.Enabled = types.BoolValue(found.Enabled)
	if found.Policy == 1 {
		config.Policy = types.StringValue("permit")
	} else {
		config.Policy = types.StringValue("deny")
	}
	config.Protocols, config.SourceIDs, config.DestinationIDs = int64DataSet(found.Protocols), stringDataSet(found.SourceIDs), stringDataSet(found.DestinationIDs)
	config.SourceType, config.DestinationType = types.Int64Value(found.SourceType), types.Int64Value(found.DestinationType)
	config.LANToWAN, config.LANToLAN = types.BoolValue(found.Direction.LANToWAN), types.BoolValue(found.Direction.LANToLAN)
	config.WANInIDs, config.VPNInIDs = stringDataSet(found.Direction.WANInIDs), stringDataSet(found.Direction.VPNInIDs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func stringDataSet(values []string) types.Set {
	elements := make([]attr.Value, len(values))
	for i, value := range values {
		elements[i] = types.StringValue(value)
	}
	return types.SetValueMust(types.StringType, elements)
}

func int64DataSet(values []int64) types.Set {
	elements := make([]attr.Value, len(values))
	for i, value := range values {
		elements[i] = types.Int64Value(value)
	}
	return types.SetValueMust(types.Int64Type, elements)
}
