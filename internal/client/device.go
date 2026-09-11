package client

import (
	"context"
	"fmt"
)

// Device is the stable inventory portion of an adopted Omada device. The
// devices endpoint also returns telemetry, but data sources deliberately expose
// only fields suitable for Terraform references.
type Device struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Model           string `json:"model"`
	CompoundModel   string `json:"compoundModel"`
	MAC             string `json:"mac"`
	SerialNumber    string `json:"sn"`
	IP              string `json:"ip"`
	Status          int64  `json:"status"`
	StatusCategory  int64  `json:"statusCategory"`
	FirmwareVersion string `json:"firmwareVersion"`
	HardwareVersion string `json:"hwVersion"`
	NeedUpgrade     bool   `json:"needUpgrade"`
	UptimeSeconds   int64  `json:"uptimeLong"`
	ClientCount     int64  `json:"clientNum"`
}

// ListDevices returns every adopted gateway, switch and access point on a site.
func (c *Client) ListDevices(ctx context.Context, siteID string) ([]Device, error) {
	var devices []Device
	path := c.classicPath(fmt.Sprintf("sites/%s/devices", siteID))
	if err := c.doAuthenticated(ctx, "GET", path, nil, nil, &devices); err != nil {
		return nil, err
	}
	return devices, nil
}
