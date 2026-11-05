package resources

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var (
	_ resource.Resource                = &siteResource{}
	_ resource.ResourceWithConfigure   = &siteResource{}
	_ resource.ResourceWithImportState = &siteResource{}
)

// siteNameMaxLength is the longest site name the controller accepts.
const siteNameMaxLength = 64

// The provider is bound to a site that has to exist before it can be
// configured, so this resource never creates or deletes one. Create takes
// over the existing site; Delete deliberately only relinquishes Terraform
// management.
type siteResource struct {
	client        *client.Client
	defaultSiteID string
}

// NewSiteResource returns the omada_site resource implementation.
func NewSiteResource() resource.Resource { return &siteResource{} }

type siteModel struct {
	ID         types.String `tfsdk:"id"`
	SiteID     types.String `tfsdk:"site_id"`
	Name       types.String `tfsdk:"name"`
	TimeZone   types.String `tfsdk:"time_zone"`
	NTPEnabled types.Bool   `tfsdk:"ntp_enabled"`
	NTPServers types.List   `tfsdk:"ntp_servers"`
}

func (r *siteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site"
}

func (r *siteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Settings of an existing Omada site: name, time zone and NTP. The site must already exist: creating this resource takes over the site, and removing it only stops Terraform management; a site is never created or deleted. An optional setting that is not configured is read but left as it is, and so is every setting this resource does not model (region, DST, scenario, location).",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, Description: "Site ID.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site_id": schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"name": schema.StringAttribute{
				Required:    true,
				Description: fmt.Sprintf("Site name, 1 to %d characters. When the provider's own site is renamed, configure the provider's site by ID: a provider configured with the previous name can no longer resolve the site.", siteNameMaxLength),
				Validators:  []validator.String{stringvalidator.UTF8LengthBetween(1, siteNameMaxLength)},
			},
			"time_zone": schema.StringAttribute{
				Optional: true, Computed: true,
				Description:   "Time zone of the site's devices, from the controller's own list (Omada Open API Access Guide, section 5.1): an IANA name, but only one per offset group, for example `Europe/Amsterdam` for Amsterdam, Berlin, Bern, Rome, Stockholm and Vienna. The controller rejects any other name, `Europe/Berlin` included, with error -1001.",
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ntp_enabled": schema.BoolAttribute{
				Optional: true, Computed: true,
				Description:   "Whether the site's devices take their time from `ntp_servers`.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"ntp_servers": schema.ListAttribute{
				ElementType: types.StringType, Optional: true, Computed: true,
				Description:   "NTP servers (host names or addresses) of the site's devices, in order.",
				Validators:    []validator.List{listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1))},
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *siteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *siteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan siteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := resourceSiteID(plan.SiteID, r.defaultSiteID)
	resp.Diagnostics.Append(r.configure(ctx, &plan, siteID)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *siteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state siteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.client.FindSite(ctx, state.SiteID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read site", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	detail, err := r.client.GetSite(ctx, found.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read site settings", err.Error())
		return
	}
	state.ID = types.StringValue(found.ID)
	resp.Diagnostics.Append(setSiteState(&state, detail)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *siteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan siteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.configure(ctx, &plan, plan.SiteID.ValueString())...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *siteResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *siteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid Import ID", "Expected the site ID or the site name as import ID, got an empty string.")
		return
	}
	siteID, err := r.client.ResolveSite(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), siteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), siteID)...)
}

// configure brings an existing site to the planned settings. A site that
// already matches is taken over without any write; otherwise the client
// rewrites the site with only the differing fields replaced, and the result
// is read back and checked.
func (r *siteResource) configure(ctx context.Context, plan *siteModel, siteID string) diag.Diagnostics {
	var diags diag.Diagnostics
	found, err := r.client.FindSite(ctx, siteID)
	if err != nil {
		diags.AddError("Unable to read site", err.Error())
		return diags
	}
	if found == nil {
		diags.AddError("Site not found", fmt.Sprintf("no site with ID %q; this resource manages an existing site and does not create one", siteID))
		return diags
	}
	current, err := r.client.GetSite(ctx, siteID)
	if err != nil {
		diags.AddError("Unable to read site settings", err.Error())
		return diags
	}

	update, changed, d := siteChanges(ctx, plan, current)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	if changed {
		if err := r.client.UpdateSite(ctx, siteID, update); err != nil {
			diags.AddError("Unable to update site", err.Error())
			return diags
		}
		if current, err = r.client.GetSite(ctx, siteID); err != nil {
			diags.AddError("Unable to read site after update", err.Error())
			return diags
		}
		if _, still, _ := siteChanges(ctx, plan, current); still {
			diags.AddError("Site not updated", fmt.Sprintf("the controller did not store the planned settings for site %s (stored: name %q, time zone %q, NTP %t %v)",
				siteID, current.Name, current.TimeZone, current.NTPEnable, current.NTPServers))
			return diags
		}
	}
	plan.ID, plan.SiteID = types.StringValue(siteID), types.StringValue(siteID)
	diags.Append(setSiteState(plan, current)...)
	return diags
}

// siteChanges compares the configured settings with the site as read. An
// unconfigured optional setting (null or unknown in the plan) never counts.
func siteChanges(ctx context.Context, plan *siteModel, current *client.SiteDetail) (client.SiteUpdate, bool, diag.Diagnostics) {
	var update client.SiteUpdate
	var diags diag.Diagnostics
	changed := false
	if name := plan.Name.ValueString(); name != current.Name {
		update.Name, changed = &name, true
	}
	if known(plan.TimeZone) {
		if tz := plan.TimeZone.ValueString(); tz != current.TimeZone {
			update.TimeZone, changed = &tz, true
		}
	}
	if known(plan.NTPEnabled) {
		if on := plan.NTPEnabled.ValueBool(); on != current.NTPEnable {
			update.NTPEnable, changed = &on, true
		}
	}
	if known(plan.NTPServers) {
		servers := []string{}
		diags.Append(plan.NTPServers.ElementsAs(ctx, &servers, false)...)
		if !slices.Equal(servers, current.NTPServers) {
			update.NTPServers, changed = servers, true
		}
	}
	return update, changed, diags
}

func known(v attr.Value) bool { return !v.IsNull() && !v.IsUnknown() }

// setSiteState copies the site as read into the model.
func setSiteState(m *siteModel, site *client.SiteDetail) diag.Diagnostics {
	servers, diags := types.ListValueFrom(context.Background(), types.StringType, site.NTPServers)
	m.Name = types.StringValue(site.Name)
	m.TimeZone = types.StringValue(site.TimeZone)
	m.NTPEnabled = types.BoolValue(site.NTPEnable)
	m.NTPServers = servers
	return diags
}
