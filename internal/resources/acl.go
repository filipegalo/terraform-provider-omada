package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var _ resource.Resource = &aclResource{}
var _ resource.ResourceWithConfigure = &aclResource{}
var _ resource.ResourceWithImportState = &aclResource{}

type aclResource struct {
	client        *client.Client
	defaultSiteID string
}

// NewACLResource returns the omada_acl resource implementation.
func NewACLResource() resource.Resource { return &aclResource{} }

type aclModel struct {
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

var aclTypeCodes = map[string]int64{"gateway": client.ACLTypeGateway, "switch": client.ACLTypeSwitch, "eap": client.ACLTypeEAP}
var aclTypeNames = map[int64]string{client.ACLTypeGateway: "gateway", client.ACLTypeSwitch: "switch", client.ACLTypeEAP: "eap"}
var aclPolicyCodes = map[string]int64{"deny": 0, "permit": 1}
var aclPolicyNames = map[int64]string{0: "deny", 1: "permit"}

func (r *aclResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acl"
}

func (r *aclResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An Omada gateway, switch or EAP firewall ACL. Updates preserve fields not modelled by this resource.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site_id": schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"type": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("gateway"), Description: "ACL surface: gateway, switch, or eap.",
				Validators: []validator.String{stringvalidator.OneOf("gateway", "switch", "eap")}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name":    schema.StringAttribute{Required: true, Description: "Rule name."},
			"enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Enable the rule."},
			"policy": schema.StringAttribute{
				Required: true, Description: "Rule action: permit or deny.", Validators: []validator.String{stringvalidator.OneOf("permit", "deny")},
			},
			"protocols": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.Int64Type, Description: "IP protocol numbers: 1 ICMP, 6 TCP, 17 UDP, or 256 all.",
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"source_type": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(0), Description: "Source entity type: 0 network, 1 IP group, or 2 port group.", Validators: []validator.Int64{int64validator.OneOf(0, 1, 2)},
			},
			"source_ids": schema.SetAttribute{Required: true, ElementType: types.StringType, Description: "Source entity IDs."},
			"destination_type": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(0), Description: "Destination entity type: 0 network, 1 IP group, or 2 port group.", Validators: []validator.Int64{int64validator.OneOf(0, 1, 2)},
			},
			"destination_ids": schema.SetAttribute{Required: true, ElementType: types.StringType, Description: "Destination entity IDs."},
			"lan_to_wan":      schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Match LAN-to-WAN traffic."},
			"lan_to_lan":      schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Match LAN-to-LAN traffic."},
			"wan_in_ids": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType, Description: "WAN interface IDs used by the direction selector.", PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"vpn_in_ids": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType, Description: "VPN IDs used by the direction selector.", PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *aclResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*client.ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *client.ProviderData, got: %T", req.ProviderData))
		return
	}
	r.client, r.defaultSiteID = data.Client, data.SiteID
}

func (r *aclResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aclModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := resourceSiteID(plan.SiteID, r.defaultSiteID)
	id, err := r.client.CreateACL(ctx, siteID, aclConfig(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to create ACL", err.Error())
		return
	}
	plan.ID, plan.SiteID = types.StringValue(id), types.StringValue(siteID)
	found, err := r.client.FindACL(ctx, siteID, aclTypeCodes[plan.Type.ValueString()], id)
	if err != nil || found == nil {
		resp.Diagnostics.AddError("Unable to read ACL after creation", readResultError(err, "ACL", id))
		return
	}
	applyACL(&plan, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *aclResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state aclModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.client.FindACL(ctx, state.SiteID.ValueString(), aclTypeCodes[state.Type.ValueString()], state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read ACL", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	applyACL(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *aclResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state aclModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateACL(ctx, state.SiteID.ValueString(), state.ID.ValueString(), aclConfig(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to update ACL", err.Error())
		return
	}
	plan.ID, plan.SiteID = state.ID, state.SiteID
	found, err := r.client.FindACL(ctx, state.SiteID.ValueString(), aclTypeCodes[plan.Type.ValueString()], state.ID.ValueString())
	if err != nil || found == nil {
		resp.Diagnostics.AddError("Unable to read ACL after update", readResultError(err, "ACL", state.ID.ValueString()))
		return
	}
	applyACL(&plan, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *aclResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state aclModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteACL(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete ACL", err.Error())
	}
}

func (r *aclResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected import ID \"site_id:type:acl_id\", got: %q", req.ID))
		return
	}
	if _, validType := aclTypeCodes[parts[1]]; !validType {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected ACL type gateway, switch, or eap, got: %q", parts[1]))
		return
	}
	siteID, err := r.client.ResolveSite(ctx, parts[0])
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), siteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
}

func aclConfig(model aclModel) client.ACLConfig {
	protocols := int64sFromSet(model.Protocols)
	if len(protocols) == 0 {
		protocols = []int64{256}
	}
	return client.ACLConfig{
		Type: aclTypeCodes[model.Type.ValueString()], Name: model.Name.ValueString(), Enabled: model.Enabled.ValueBool(), Policy: aclPolicyCodes[model.Policy.ValueString()],
		Protocols: protocols, SourceType: model.SourceType.ValueInt64(), SourceIDs: stringsFromSet(model.SourceIDs), DestinationType: model.DestinationType.ValueInt64(), DestinationIDs: stringsFromSet(model.DestinationIDs),
		Direction: client.ACLDirection{LANToWAN: model.LANToWAN.ValueBool(), LANToLAN: model.LANToLAN.ValueBool(), WANInIDs: stringsFromSet(model.WANInIDs), VPNInIDs: stringsFromSet(model.VPNInIDs)},
	}
}

func applyACL(state *aclModel, acl *client.ACL) {
	state.ID = types.StringValue(acl.ID)
	state.Type = types.StringValue(aclTypeNames[acl.Type])
	state.Name = types.StringValue(acl.Name)
	state.Enabled = types.BoolValue(acl.Enabled)
	state.Policy = types.StringValue(aclPolicyNames[acl.Policy])
	state.Protocols = int64Set(acl.Protocols)
	state.SourceType = types.Int64Value(acl.SourceType)
	state.SourceIDs = stringSet(acl.SourceIDs)
	state.DestinationType = types.Int64Value(acl.DestinationType)
	state.DestinationIDs = stringSet(acl.DestinationIDs)
	state.LANToWAN = types.BoolValue(acl.Direction.LANToWAN)
	state.LANToLAN = types.BoolValue(acl.Direction.LANToLAN)
	state.WANInIDs = stringSet(acl.Direction.WANInIDs)
	state.VPNInIDs = stringSet(acl.Direction.VPNInIDs)
}

func int64Set(values []int64) types.Set {
	elements := make([]attr.Value, len(values))
	for i, value := range values {
		elements[i] = types.Int64Value(value)
	}
	return types.SetValueMust(types.Int64Type, elements)
}

func int64sFromSet(value types.Set) []int64 {
	elements := value.Elements()
	result := make([]int64, 0, len(elements))
	for _, element := range elements {
		if number, ok := element.(types.Int64); ok {
			result = append(result, number.ValueInt64())
		}
	}
	return result
}
