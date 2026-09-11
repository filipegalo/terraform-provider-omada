package resources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var (
	_ resource.Resource                = &switchPortResource{}
	_ resource.ResourceWithConfigure   = &switchPortResource{}
	_ resource.ResourceWithImportState = &switchPortResource{}
)

// A physical port cannot be created or deleted. Create configures the selected
// existing port; Delete deliberately only relinquishes Terraform management.
type switchPortResource struct {
	client        *client.Client
	defaultSiteID string
}

func NewSwitchPortResource() resource.Resource { return &switchPortResource{} }

type switchPortModel struct {
	ID                        types.String `tfsdk:"id"`
	SiteID                    types.String `tfsdk:"site_id"`
	SwitchMAC                 types.String `tfsdk:"switch_mac"`
	Port                      types.Int64  `tfsdk:"port"`
	Name                      types.String `tfsdk:"name"`
	TagIDs                    types.Set    `tfsdk:"tag_ids"`
	NativeNetworkID           types.String `tfsdk:"native_network_id"`
	NetworkTagsSetting        types.Int64  `tfsdk:"network_tags_setting"`
	ProfileID                 types.String `tfsdk:"profile_id"`
	ProfileOverrideEnable     types.Bool   `tfsdk:"profile_override_enable"`
	ProfileVLANOverrideEnable types.Bool   `tfsdk:"profile_vlan_override_enable"`
	LinkSpeed                 types.Int64  `tfsdk:"link_speed"`
	Duplex                    types.Int64  `tfsdk:"duplex"`
}

func (r *switchPortResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_switch_port"
}

func (r *switchPortResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	computedString := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, Description: description, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}
	}
	resp.Schema = schema.Schema{
		Description: "Configuration of an existing physical port on an Omada-managed switch. Removing this resource only stops Terraform management; it does not reset the live port.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site_id": schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"switch_mac": schema.StringAttribute{
				Required:    true,
				Description: "MAC address of the managed switch, using either colon or dash separators.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`(?i)^(?:(?:[0-9a-f]{2}:){5}[0-9a-f]{2}|(?:[0-9a-f]{2}-){5}[0-9a-f]{2})$`), "must be a six-byte MAC address using consistent colon or dash separators"),
				},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"port":              schema.Int64Attribute{Required: true, Description: "Physical port number printed by the switch, starting at 1.", Validators: []validator.Int64{int64validator.AtLeast(1)}, PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"name":              computedString("Port display name."),
			"tag_ids":           schema.SetAttribute{Optional: true, Computed: true, Description: "Controller tag IDs applied to the port (these are labels, not VLAN IDs).", ElementType: types.StringType, PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()}},
			"native_network_id": computedString("ID of the untagged/native VLAN network."),
			"network_tags_setting": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "VLAN tag policy: 0 Allow All or 1 Block All. Custom (2) can be read from the controller but must be configured through profile_id because the safe port PATCH does not accept tagged-network lists.",
				Validators:  []validator.Int64{int64validator.OneOf(0, 1)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"profile_id":                   computedString("ID of the switch LAN/port profile assigned to the port."),
			"profile_override_enable":      schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether profile-level port settings are overridden."},
			"profile_vlan_override_enable": schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether the profile VLAN settings are overridden."},
			"link_speed": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Controller link-speed enum, where 0 is Auto. Other supported values depend on the switch model.",
				Validators:  []validator.Int64{int64validator.AtLeast(0)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"duplex": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Duplex enum: 0 Auto, 1 Half, 2 Full.",
				Validators:  []validator.Int64{int64validator.OneOf(0, 1, 2)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *switchPortResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *switchPortResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan switchPortModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := resourceSiteID(plan.SiteID, r.defaultSiteID)
	if err := r.configure(ctx, &plan, siteID); err != nil {
		resp.Diagnostics.AddError("Unable to configure switch port", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *switchPortResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state switchPortModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.client.FindSwitchPort(ctx, state.SiteID.ValueString(), state.SwitchMAC.ValueString(), state.Port.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read switch port", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	applySwitchPort(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *switchPortResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan switchPortModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.configure(ctx, &plan, plan.SiteID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to update switch port", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *switchPortResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *switchPortResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	siteNameOrID, rest, ok := strings.Cut(req.ID, ":")
	switchMAC, portText, portOK := strings.Cut(rest, ":")
	port, err := strconv.ParseInt(portText, 10, 64)
	if !ok || !portOK || siteNameOrID == "" || switchMAC == "" || err != nil || port < 1 {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected import ID \"site_id:switch_mac:port\", got: %q", req.ID))
		return
	}
	siteID, err := r.client.ResolveSite(ctx, siteNameOrID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	switchMAC = client.NormalizeSwitchMAC(switchMAC)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), siteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("switch_mac"), switchMAC)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("port"), port)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), switchPortID(siteID, switchMAC, port))...)
}

func (r *switchPortResource) configure(ctx context.Context, plan *switchPortModel, siteID string) error {
	configuredMAC := plan.SwitchMAC.ValueString()
	mac := client.NormalizeSwitchMAC(configuredMAC)
	found, err := r.client.FindSwitchPort(ctx, siteID, mac, plan.Port.ValueInt64())
	if err != nil {
		return err
	}
	if found == nil {
		return fmt.Errorf("switch %s has no physical port %d", mac, plan.Port.ValueInt64())
	}
	if err := r.client.UpdateSwitchPort(ctx, siteID, mac, found.Port, switchPortConfig(*plan, found)); err != nil {
		return err
	}
	updated, err := r.client.FindSwitchPort(ctx, siteID, mac, found.Port)
	if err != nil {
		return fmt.Errorf("reading switch port after update: %w", err)
	}
	if updated == nil {
		return fmt.Errorf("switch port %d was not returned after update", found.Port)
	}
	plan.SiteID = types.StringValue(siteID)
	// switch_mac is configured and therefore must remain byte-for-byte equal
	// to the planned value. Only API paths and the computed ID are canonical.
	plan.SwitchMAC = types.StringValue(configuredMAC)
	applySwitchPort(plan, updated)
	return nil
}

func switchPortConfig(model switchPortModel, found *client.SwitchPort) client.SwitchPortConfig {
	return client.SwitchPortConfig{
		Name: pickString(model.Name, found.Name), TagIDs: pickStringSet(model.TagIDs, found.TagIDs),
		NativeNetworkID: pickString(model.NativeNetworkID, found.NativeNetworkID), NetworkTagsSetting: pickInt(model.NetworkTagsSetting, found.NetworkTagsSetting),
		ProfileID: pickString(model.ProfileID, found.ProfileID), ProfileOverrideEnable: pickBool(model.ProfileOverrideEnable, found.ProfileOverrideEnable),
		ProfileVLANOverrideEnable: pickBool(model.ProfileVLANOverrideEnable, found.ProfileVLANOverrideEnable), LinkSpeed: pickInt(model.LinkSpeed, found.LinkSpeed), Duplex: pickInt(model.Duplex, found.Duplex),
	}
}

func applySwitchPort(state *switchPortModel, found *client.SwitchPort) {
	state.ID = types.StringValue(switchPortID(state.SiteID.ValueString(), state.SwitchMAC.ValueString(), found.Port))
	state.Port, state.Name = types.Int64Value(found.Port), types.StringValue(found.Name)
	state.TagIDs, state.NativeNetworkID = stringSet(found.TagIDs), types.StringValue(found.NativeNetworkID)
	state.NetworkTagsSetting, state.ProfileID = types.Int64Value(found.NetworkTagsSetting), types.StringValue(found.ProfileID)
	state.ProfileOverrideEnable, state.ProfileVLANOverrideEnable = types.BoolValue(found.ProfileOverrideEnable), types.BoolValue(found.ProfileVLANOverrideEnable)
	state.LinkSpeed, state.Duplex = types.Int64Value(found.LinkSpeed), types.Int64Value(found.Duplex)
}

func resourceSiteID(site types.String, fallback string) string {
	if site.IsNull() || site.IsUnknown() {
		return fallback
	}
	return site.ValueString()
}
func pickString(value types.String, fallback string) string {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return value.ValueString()
}
func pickInt(value types.Int64, fallback int64) int64 {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return value.ValueInt64()
}
func pickBool(value types.Bool, fallback bool) bool {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return value.ValueBool()
}
func pickStringSet(value types.Set, fallback []string) []string {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return stringsFromSet(value)
}
func stringSet(values []string) types.Set {
	return types.SetValueMust(types.StringType, stringsToValues(values))
}
func stringsToValues(values []string) []attr.Value {
	result := make([]attr.Value, len(values))
	for i, value := range values {
		result[i] = types.StringValue(value)
	}
	return result
}
func stringsFromSet(value types.Set) []string {
	elements := value.Elements()
	result := make([]string, 0, len(elements))
	for _, element := range elements {
		if text, ok := element.(types.String); ok {
			result = append(result, text.ValueString())
		}
	}
	return result
}
func switchPortID(siteID, switchMAC string, port int64) string {
	return fmt.Sprintf("%s:%s:%d", siteID, client.NormalizeSwitchMAC(switchMAC), port)
}
