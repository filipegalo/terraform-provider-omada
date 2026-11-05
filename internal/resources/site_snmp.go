package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var (
	_ resource.Resource                = &siteSNMPResource{}
	_ resource.ResourceWithConfigure   = &siteSNMPResource{}
	_ resource.ResourceWithImportState = &siteSNMPResource{}
)

// Every site has exactly one SNMP setting, so like omada_site this resource
// takes over the existing setting on create and only relinquishes it on
// delete. Credentials are deliberately not modelled: they would land in the
// state, and the controller keeps them while a version is switched off.
type siteSNMPResource struct {
	client        *client.Client
	defaultSiteID string
}

// NewSiteSNMPResource returns the omada_site_snmp resource implementation.
func NewSiteSNMPResource() resource.Resource { return &siteSNMPResource{} }

type siteSNMPModel struct {
	ID           types.String `tfsdk:"id"`
	SiteID       types.String `tfsdk:"site_id"`
	V1V2CEnabled types.Bool   `tfsdk:"v1v2c_enabled"`
	V3Enabled    types.Bool   `tfsdk:"v3_enabled"`
}

func (r *siteSNMPResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site_snmp"
}

func (r *siteSNMPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Which SNMP versions the devices of an existing Omada site answer. Creating this resource takes over the site's SNMP setting, and removing it only stops Terraform management. The community string and the SNMPv3 credentials are not managed: switching a version on needs them to be set in the controller first, and switching it off keeps them there.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, Description: "Site ID.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site_id": schema.StringAttribute{Optional: true, Computed: true, Description: "Site ID. Defaults to the provider site.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"v1v2c_enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Description: "SNMP v1 and v2c.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"v3_enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Description: "SNMPv3.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *siteSNMPResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *siteSNMPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan siteSNMPModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.configure(ctx, &plan, resourceSiteID(plan.SiteID, r.defaultSiteID)); err != nil {
		resp.Diagnostics.AddError("Unable to configure SNMP", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *siteSNMPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state siteSNMPModel
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
	snmp, err := r.client.GetSiteSNMP(ctx, found.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read SNMP", err.Error())
		return
	}
	state.ID = types.StringValue(found.ID)
	setSiteSNMPState(&state, snmp)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *siteSNMPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan siteSNMPModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.configure(ctx, &plan, plan.SiteID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to update SNMP", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *siteSNMPResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *siteSNMPResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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

// configure sends only the configured switches that differ from the setting
// as read, then reads it back and checks it.
func (r *siteSNMPResource) configure(ctx context.Context, plan *siteSNMPModel, siteID string) error {
	current, err := r.client.GetSiteSNMP(ctx, siteID)
	if err != nil {
		return err
	}
	update, changed := siteSNMPChanges(plan, current)
	if changed {
		if err := r.client.UpdateSiteSNMP(ctx, siteID, update); err != nil {
			return err
		}
		if current, err = r.client.GetSiteSNMP(ctx, siteID); err != nil {
			return fmt.Errorf("reading SNMP after update: %w", err)
		}
		if _, still := siteSNMPChanges(plan, current); still {
			return fmt.Errorf("the controller stored v1/v2c %t and v3 %t instead of the planned values", current.V1V2CEnable, current.V3Enable)
		}
	}
	plan.ID, plan.SiteID = types.StringValue(siteID), types.StringValue(siteID)
	setSiteSNMPState(plan, current)
	return nil
}

func siteSNMPChanges(plan *siteSNMPModel, current *client.SiteSNMP) (client.SiteSNMP, bool) {
	update := *current
	if known(plan.V1V2CEnabled) {
		update.V1V2CEnable = plan.V1V2CEnabled.ValueBool()
	}
	if known(plan.V3Enabled) {
		update.V3Enable = plan.V3Enabled.ValueBool()
	}
	return update, update != *current
}

func setSiteSNMPState(m *siteSNMPModel, snmp *client.SiteSNMP) {
	m.V1V2CEnabled = types.BoolValue(snmp.V1V2CEnable)
	m.V3Enabled = types.BoolValue(snmp.V3Enable)
}
