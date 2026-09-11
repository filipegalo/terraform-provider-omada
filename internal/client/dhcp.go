package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const dhcpReservationEndpoint = "setting/service/dhcp"

// DHCPReservation is a DHCP static IP reservation as stored on the
// controller. Confirmed live, from the real Omada web UI's own edit
// request: updating a reservation is a full-object PUT, not a partial
// patch, so this struct carries every field that request round-trips
// (Options and FeatureDescription are opaque server-managed data this
// provider never sets, kept as raw JSON so an update echoes them back
// unchanged instead of guessing their shape).
type DHCPReservation struct {
	MAC                string          `json:"mac"`
	IP                 string          `json:"ip"`
	NetID              string          `json:"netId"`
	Status             bool            `json:"status"`
	Description        string          `json:"description,omitempty"`
	Name               string          `json:"name,omitempty"`
	ServerMAC          string          `json:"serverMac,omitempty"`
	ServerType         string          `json:"serverType,omitempty"`
	Options            json.RawMessage `json:"options,omitempty"`
	FeatureDescription json.RawMessage `json:"featureDescription,omitempty"`
}

// dhcpBasePath builds the classic (session-authenticated) API path for a
// site's DHCP reservations. There's an identically-shaped path under
// /openapi/v1/... (TP-Link's actual "Open API" product), but that requires
// separate client ID/secret application credentials this provider doesn't
// set up; this classic path works with the same session login the rest of
// the client already uses.
func (c *Client) dhcpBasePath(siteID string) string {
	return c.classicPath(fmt.Sprintf("sites/%s/%s", siteID, dhcpReservationEndpoint))
}

// ListDHCPReservations returns every DHCP reservation configured on the
// given site.
func (c *Client) ListDHCPReservations(ctx context.Context, siteID string) ([]DHCPReservation, error) {
	return listAllPages[DHCPReservation](ctx, c, c.dhcpBasePath(siteID))
}

// normalizeMAC strips separators and lowercases a MAC address so two
// addresses can be compared regardless of notation. Confirmed live: this
// classic endpoint's GET responses use dash-separated MACs
// ("AA-BB-CC-DD-EE-FF") while every other input to this provider (resource
// config, import IDs) uses colons -- a bare case-insensitive string compare
// between the two never matches.
func normalizeMAC(mac string) string {
	return strings.ToLower(strings.NewReplacer(":", "", "-", "").Replace(mac))
}

// FindDHCPReservationByMAC looks up a reservation by MAC address, matched
// regardless of case or separator (":" vs "-"). There is no get-by-MAC
// endpoint, so this lists and filters. A nil, nil return means no
// reservation exists for that MAC.
func (c *Client) FindDHCPReservationByMAC(ctx context.Context, siteID, mac string) (*DHCPReservation, error) {
	reservations, err := c.ListDHCPReservations(ctx, siteID)
	if err != nil {
		return nil, err
	}
	target := normalizeMAC(mac)
	for _, r := range reservations {
		if normalizeMAC(r.MAC) == target {
			return &r, nil
		}
	}
	return nil, nil
}

// CreateDHCPReservationRequest is the body for creating a reservation.
type CreateDHCPReservationRequest struct {
	MAC         string `json:"mac"`
	IP          string `json:"ip"`
	NetID       string `json:"netId"`
	Status      bool   `json:"status"`
	Description string `json:"description,omitempty"`
	Name        string `json:"name,omitempty"`
}

// CreateDHCPReservation creates a new static IP reservation. Name is always
// set equal to Description: left unset, the controller auto-generates its
// own name (observed live: an odd, differently-cased, differently-punctuated
// string unrelated to what was asked for), and there is no reason for this
// provider's two labels for the same reservation to ever disagree.
func (c *Client) CreateDHCPReservation(ctx context.Context, siteID string, req CreateDHCPReservationRequest) error {
	req.Name = req.Description
	return c.doAuthenticated(ctx, "POST", c.dhcpBasePath(siteID), nil, req, nil)
}

// UpdateDHCPReservationRequest carries only the fields the caller wants to
// change; nil fields keep whatever the reservation already has.
type UpdateDHCPReservationRequest struct {
	IP          *string
	Description *string
	Status      *bool
}

// UpdateDHCPReservation changes an existing reservation, identified by MAC.
// Confirmed live, from the real Omada web UI's own edit request: this
// classic endpoint is a full-object PUT keyed by the reservation's MAC in
// the URL (dash-separated, exactly as the controller reports it -- not the
// reservation's own internal id, and not colon-separated), not a partial
// PATCH. So the current object is read first and only the requested fields
// are changed before writing the whole thing back; nothing else the
// controller manages (serverMac, options, ...) gets clobbered.
//
// Name is unconditionally set equal to the resulting Description on every
// update, even when Description isn't part of this particular change: this
// self-heals any reservation the controller previously auto-named on
// create (see CreateDHCPReservation) the moment it's touched again.
func (c *Client) UpdateDHCPReservation(ctx context.Context, siteID, mac string, req UpdateDHCPReservationRequest) error {
	existing, err := c.FindDHCPReservationByMAC(ctx, siteID, mac)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("no DHCP reservation found for MAC %q on site %q", mac, siteID)
	}

	updated := *existing
	if req.IP != nil {
		updated.IP = *req.IP
	}
	if req.Description != nil {
		updated.Description = *req.Description
	}
	if req.Status != nil {
		updated.Status = *req.Status
	}
	updated.Name = updated.Description

	path := fmt.Sprintf("%s/%s", c.dhcpBasePath(siteID), existing.MAC)
	return c.doAuthenticated(ctx, "PUT", path, nil, updated, nil)
}

// DeleteDHCPReservation deletes a reservation, identified by MAC. Keyed in
// the URL the same way UpdateDHCPReservation is: by the exact MAC string
// the controller itself reports for this reservation, not a
// caller-supplied or reformatted one. A missing reservation is treated as
// already deleted.
func (c *Client) DeleteDHCPReservation(ctx context.Context, siteID, mac string) error {
	existing, err := c.FindDHCPReservationByMAC(ctx, siteID, mac)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}
	path := fmt.Sprintf("%s/%s", c.dhcpBasePath(siteID), existing.MAC)
	return c.doAuthenticated(ctx, "DELETE", path, nil, nil, nil)
}
