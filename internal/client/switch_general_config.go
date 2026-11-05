package client

import (
	"context"
	"fmt"
)

const (
	// errSwitchNotFound is the Open API's answer for a MAC that is not an
	// adopted switch on the site.
	errSwitchNotFound = -39700
	// errAgileSwitchPath is the Open API's answer when an Agile Series (Easy
	// Managed, ES...) switch is addressed through the path of the other switch
	// series: "The Agile Series Switch should use the corresponding path."
	errAgileSwitchPath = -39742

	// SwitchNameMaxLength is the longest device name a switch accepts.
	SwitchNameMaxLength = 128
	// EasyManagedSwitchNameMaxLength is the longest device name an Agile
	// Series (Easy Managed) switch accepts.
	EasyManagedSwitchNameMaxLength = 32
)

// SwitchGeneralConfig is the part of a switch's general configuration this
// provider models. The endpoint also returns LED, jumbo-frame, LAG, SNMP and
// location settings, which differ between the two switch series and are
// deliberately left to the controller.
type SwitchGeneralConfig struct {
	Name string `json:"name"`
	// EasyManaged is not part of the response. It records which of the two
	// general-config paths answered, so that a write goes to the same one.
	EasyManaged bool `json:"-"`
}

// SwitchGeneralConfigUpdate is a partial general-config PATCH. Confirmed live
// against 6.3.0.45: the controller changes only the fields present in the
// body, so a nil field is omitted and keeps its current value.
type SwitchGeneralConfigUpdate struct {
	Name *string `json:"name,omitempty"`
}

// switchGeneralConfigPath builds the Open API path of a switch's general
// configuration. Agile Series switches live below an extra "es" segment.
func (c *Client) switchGeneralConfigPath(siteID, switchMAC string, easyManaged bool) string {
	series := ""
	if easyManaged {
		series = "es/"
	}
	return c.openAPIPath("v1", siteID, fmt.Sprintf("switches/%s%s/general-config", series, NormalizeSwitchMAC(switchMAC)))
}

// GetSwitchGeneralConfig reads the general configuration of an adopted switch.
// A nil, nil return means the site has no switch with that MAC.
//
// The series is taken from the controller's own answer rather than guessed
// from the model name: the regular path rejects an Agile Series switch with a
// dedicated error code, and only then is the "es" path used. The order
// matters, because the "es" path also answers for a regular switch, with the
// reduced Easy Managed field set.
func (c *Client) GetSwitchGeneralConfig(ctx context.Context, siteID, switchMAC string) (*SwitchGeneralConfig, error) {
	var cfg SwitchGeneralConfig
	err := c.doPublicOpenAPI(ctx, "GET", c.switchGeneralConfigPath(siteID, switchMAC, false), nil, &cfg)
	if isAPIError(err, errAgileSwitchPath) {
		cfg = SwitchGeneralConfig{EasyManaged: true}
		err = c.doPublicOpenAPI(ctx, "GET", c.switchGeneralConfigPath(siteID, switchMAC, true), nil, &cfg)
	}
	if isAPIError(err, errSwitchNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpdateSwitchGeneralConfig patches the general configuration of an adopted
// switch. easyManaged selects the path and must come from a preceding
// GetSwitchGeneralConfig. Only the fields set in update are sent.
func (c *Client) UpdateSwitchGeneralConfig(ctx context.Context, siteID, switchMAC string, easyManaged bool, update SwitchGeneralConfigUpdate) error {
	return c.doPublicOpenAPI(ctx, "PATCH", c.switchGeneralConfigPath(siteID, switchMAC, easyManaged), update, nil)
}
