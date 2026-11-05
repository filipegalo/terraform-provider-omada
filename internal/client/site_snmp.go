package client

import "context"

// SiteSNMP is the part of a site's SNMP service setting this provider models:
// the two switches. The endpoint also returns the community string and the
// SNMPv3 credentials; they are not decoded, so they never reach the state.
type SiteSNMP struct {
	V1V2CEnable bool `json:"snmpV1V2CEnable"`
	V3Enable    bool `json:"snmpV3Enable"`
}

func (c *Client) siteSNMPPath(siteID string) string {
	return c.openAPIPath("v1", siteID, "setting/service/snmp")
}

// GetSiteSNMP reads which SNMP versions a site's devices answer.
func (c *Client) GetSiteSNMP(ctx context.Context, siteID string) (*SiteSNMP, error) {
	var snmp SiteSNMP
	if err := c.doPublicOpenAPI(ctx, "GET", c.siteSNMPPath(siteID), nil, &snmp); err != nil {
		return nil, err
	}
	return &snmp, nil
}

// UpdateSiteSNMP switches SNMP versions on or off. The PATCH needs both
// switches (6.3.0.111: "Parameter [snmpV1V2CEnable] should not be null") and
// leaves the credentials alone when they are omitted. Turning a version on
// needs its credentials, which have to be set in the controller first.
func (c *Client) UpdateSiteSNMP(ctx context.Context, siteID string, snmp SiteSNMP) error {
	return c.doPublicOpenAPI(ctx, "PATCH", c.siteSNMPPath(siteID), snmp, nil)
}
