// Package resources implements the omada provider's Terraform resources.
package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

var (
	_ resource.Resource                = &dhcpReservationResource{}
	_ resource.ResourceWithConfigure   = &dhcpReservationResource{}
	_ resource.ResourceWithImportState = &dhcpReservationResource{}
)

type dhcpReservationResource struct {
	client *client.Client
	// defaultSiteID is the provider's resolved site, used when a resource
	// doesn't set site_id explicitly.
	defaultSiteID string
}

// NewDHCPReservationResource is the resource.Resource factory for
// omada_dhcp_reservation.
func NewDHCPReservationResource() resource.Resource {
	return &dhcpReservationResource{}
}

type dhcpReservationModel struct {
	ID          types.String `tfsdk:"id"`
	SiteID      types.String `tfsdk:"site_id"`
	NetworkID   types.String `tfsdk:"network_id"`
	MACAddress  types.String `tfsdk:"mac_address"`
	IPAddress   types.String `tfsdk:"ip_address"`
	Description types.String `tfsdk:"description"`
	Enabled     types.Bool   `tfsdk:"enabled"`
}

func (r *dhcpReservationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dhcp_reservation"
}

func (r *dhcpReservationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A DHCP static IP reservation on an Omada Controller site.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Synthesized as \"{site_id}:{mac_address}\".",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "ID of the site this reservation belongs to. Defaults to the provider's configured site; set explicitly to target a different site from this provider instance.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"network_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the LAN network the reservation's IP address belongs to. Not updatable in place: the DHCP reservation API only patches ip/description/status.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mac_address": schema.StringAttribute{
				Required:    true,
				Description: "MAC address of the device to reserve an IP for. Together with site_id, this is the reservation's real identity.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ip_address": schema.StringAttribute{
				Required:    true,
				Description: "IP address to reserve for the device.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Free-text description of the reservation.",
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether the reservation is active.",
			},
		},
	}
}

func (r *dhcpReservationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data, ok := req.ProviderData.(*client.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.ProviderData, got: %T", req.ProviderData),
		)
		return
	}
	r.client = data.Client
	r.defaultSiteID = data.SiteID
}

func (r *dhcpReservationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dhcpReservationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := plan.SiteID.ValueString()
	if plan.SiteID.IsNull() || plan.SiteID.IsUnknown() {
		siteID = r.defaultSiteID
	}

	err := r.client.CreateDHCPReservation(ctx, siteID, client.CreateDHCPReservationRequest{
		MAC:         plan.MACAddress.ValueString(),
		IP:          plan.IPAddress.ValueString(),
		NetID:       plan.NetworkID.ValueString(),
		Status:      plan.Enabled.ValueBool(),
		Description: plan.Description.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create DHCP reservation", err.Error())
		return
	}

	plan.SiteID = types.StringValue(siteID)
	plan.ID = types.StringValue(reservationID(siteID, plan.MACAddress.ValueString()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dhcpReservationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dhcpReservationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, err := r.client.FindDHCPReservationByMAC(ctx, state.SiteID.ValueString(), state.MACAddress.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read DHCP reservation", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.IPAddress = types.StringValue(found.IP)
	state.NetworkID = types.StringValue(found.NetID)
	state.Enabled = types.BoolValue(found.Status)
	if found.Description != "" {
		state.Description = types.StringValue(found.Description)
	} else {
		state.Description = types.StringNull()
	}
	state.ID = types.StringValue(reservationID(state.SiteID.ValueString(), state.MACAddress.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dhcpReservationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dhcpReservationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var patch client.UpdateDHCPReservationRequest
	if !plan.IPAddress.Equal(state.IPAddress) {
		v := plan.IPAddress.ValueString()
		patch.IP = &v
	}
	if !plan.Description.Equal(state.Description) {
		v := plan.Description.ValueString()
		patch.Description = &v
	}
	if !plan.Enabled.Equal(state.Enabled) {
		v := plan.Enabled.ValueBool()
		patch.Status = &v
	}

	if err := r.client.UpdateDHCPReservation(ctx, plan.SiteID.ValueString(), plan.MACAddress.ValueString(), patch); err != nil {
		resp.Diagnostics.AddError("Unable to update DHCP reservation", err.Error())
		return
	}

	plan.ID = types.StringValue(reservationID(plan.SiteID.ValueString(), plan.MACAddress.ValueString()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dhcpReservationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dhcpReservationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteDHCPReservation(ctx, state.SiteID.ValueString(), state.MACAddress.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete DHCP reservation", err.Error())
	}
}

func (r *dhcpReservationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	siteNameOrID, mac, ok := strings.Cut(req.ID, ":")
	if !ok || siteNameOrID == "" || mac == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected import ID in the form \"site_id:mac_address\" (site_id may also be a site name), got: %q", req.ID),
		)
		return
	}

	// Accept a site name as well as an ID: newer Omada UIs don't always
	// surface the site ID in the URL, but the name is always visible in the
	// site selector.
	siteID, err := r.client.ResolveSite(ctx, siteNameOrID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), siteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac_address"), mac)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), reservationID(siteID, mac))...)
}

func reservationID(siteID, mac string) string {
	return fmt.Sprintf("%s:%s", siteID, strings.ToLower(mac))
}
