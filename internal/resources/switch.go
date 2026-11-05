package resources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var (
	_ resource.Resource                = &switchResource{}
	_ resource.ResourceWithConfigure   = &switchResource{}
	_ resource.ResourceWithImportState = &switchResource{}
)

var switchMACPattern = regexp.MustCompile(`(?i)^(?:(?:[0-9a-f]{2}:){5}[0-9a-f]{2}|(?:[0-9a-f]{2}-){5}[0-9a-f]{2})$`)

// A switch joins a site by being adopted in the controller, not by Terraform.
// Create takes over the general configuration of an adopted switch; Delete
// deliberately only relinquishes Terraform management.
type switchResource struct {
	client        *client.Client
	defaultSiteID string
}

// NewSwitchResource returns the omada_switch resource implementation.
func NewSwitchResource() resource.Resource { return &switchResource{} }

type switchModel struct {
	ID     types.String `tfsdk:"id"`
	SiteID types.String `tfsdk:"site_id"`
	MAC    types.String `tfsdk:"mac"`
	Name   types.String `tfsdk:"name"`
}

func (r *switchResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_switch"
}

func (r *switchResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "General configuration of an existing Omada-managed switch, currently its name. The switch must already be adopted by the controller: creating this resource takes over the adopted switch, and removing it only stops Terraform management; it does not forget or reset the switch. Both regular and Agile Series (Easy Managed) switches are supported.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site_id": schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"mac": schema.StringAttribute{
				Required:    true,
				Description: "MAC address of the adopted switch, using either colon or dash separators.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(switchMACPattern, "must be a six-byte MAC address using consistent colon or dash separators"),
				},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: fmt.Sprintf("Device name shown by the controller, 1 to %d characters. Agile Series (Easy Managed) switches accept at most %d characters.", client.SwitchNameMaxLength, client.EasyManagedSwitchNameMaxLength),
				Validators:  []validator.String{stringvalidator.UTF8LengthBetween(1, client.SwitchNameMaxLength)},
			},
		},
	}
}

func (r *switchResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *switchResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan switchModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := resourceSiteID(plan.SiteID, r.defaultSiteID)
	if err := r.configure(ctx, &plan, siteID); err != nil {
		resp.Diagnostics.AddError("Unable to configure switch", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *switchResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state switchModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.client.GetSwitchGeneralConfig(ctx, state.SiteID.ValueString(), state.MAC.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read switch", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.ID = types.StringValue(switchID(state.SiteID.ValueString(), state.MAC.ValueString()))
	state.Name = types.StringValue(found.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *switchResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan switchModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.configure(ctx, &plan, plan.SiteID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to update switch", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *switchResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *switchResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	siteNameOrID, switchMAC, ok := splitSwitchImportID(req.ID)
	if !ok {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected import ID \"switch_mac\" or \"site_id:switch_mac\", got: %q", req.ID))
		return
	}
	siteID := r.defaultSiteID
	if siteNameOrID != "" {
		resolved, err := r.client.ResolveSite(ctx, siteNameOrID)
		if err != nil {
			resp.Diagnostics.AddError("Unable to resolve site", err.Error())
			return
		}
		siteID = resolved
	}
	switchMAC = client.NormalizeSwitchMAC(switchMAC)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), siteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), switchMAC)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), switchID(siteID, switchMAC))...)
}

// configure brings an adopted switch to the planned name. The name is the only
// field ever written: the controller's general-config PATCH is partial, so the
// switch's LED, jumbo-frame, LAG, SNMP and location settings are not touched.
// A switch that already carries the name is taken over without any write.
func (r *switchResource) configure(ctx context.Context, plan *switchModel, siteID string) error {
	configuredMAC := plan.MAC.ValueString()
	mac := client.NormalizeSwitchMAC(configuredMAC)
	name := plan.Name.ValueString()
	found, err := r.client.GetSwitchGeneralConfig(ctx, siteID, mac)
	if err != nil {
		return err
	}
	if found == nil {
		return fmt.Errorf("site %s has no adopted switch with MAC %s", siteID, mac)
	}
	if err := checkSwitchName(name, found.EasyManaged); err != nil {
		return err
	}
	if found.Name != name {
		if err := r.client.UpdateSwitchGeneralConfig(ctx, siteID, mac, found.EasyManaged, client.SwitchGeneralConfigUpdate{Name: &name}); err != nil {
			return err
		}
		updated, err := r.client.GetSwitchGeneralConfig(ctx, siteID, mac)
		if err != nil {
			return fmt.Errorf("reading switch after update: %w", err)
		}
		if updated == nil {
			return fmt.Errorf("switch %s was not returned after update", mac)
		}
		if updated.Name != name {
			return fmt.Errorf("the controller stored the switch name %q instead of %q", updated.Name, name)
		}
	}
	plan.SiteID = types.StringValue(siteID)
	// mac is configured and therefore must remain byte-for-byte equal to the
	// planned value. Only API paths and the computed ID are canonical.
	plan.ID = types.StringValue(switchID(siteID, configuredMAC))
	return nil
}

// checkSwitchName enforces the shorter name limit of the Agile Series, which
// is only known once the controller has told the two switch series apart and
// so cannot be a schema validator.
func checkSwitchName(name string, easyManaged bool) error {
	if length := utf8.RuneCountInString(name); easyManaged && length > client.EasyManagedSwitchNameMaxLength {
		return fmt.Errorf("the name has %d characters, but Agile Series (Easy Managed) switches accept at most %d", length, client.EasyManagedSwitchNameMaxLength)
	}
	return nil
}

// splitSwitchImportID accepts a bare MAC, which selects the provider site, or
// "site:mac". Because a colon-separated MAC contains the separator itself,
// the whole ID is tried as a MAC before it is split.
func splitSwitchImportID(id string) (siteNameOrID, switchMAC string, ok bool) {
	if switchMACPattern.MatchString(id) {
		return "", id, true
	}
	siteNameOrID, switchMAC, found := strings.Cut(id, ":")
	if !found || siteNameOrID == "" || !switchMACPattern.MatchString(switchMAC) {
		return "", "", false
	}
	return siteNameOrID, switchMAC, true
}

func switchID(siteID, switchMAC string) string {
	return fmt.Sprintf("%s:%s", siteID, client.NormalizeSwitchMAC(switchMAC))
}
