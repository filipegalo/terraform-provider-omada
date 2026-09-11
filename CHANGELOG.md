# Changelog

## 0.1.0 (Unreleased)

First release.

FEATURES:

* **New Resource:** `omada_dhcp_reservation` — static DHCP reservations on a
  LAN network, keyed by MAC address.
* **New Resource:** `omada_vlan` — gateway-backed VLAN interfaces, with their
  IPv4 gateway, DHCP scope and inter-VLAN isolation.
* provider: authenticates against a controller's local API with a local admin
  account; no Omada Cloud account or API client credentials required. Every
  setting can come from an environment variable (`OMADA_URL`,
  `OMADA_USERNAME`, `OMADA_PASSWORD`, `OMADA_SITE`, `OMADA_SKIP_TLS_VERIFY`).
* provider: `site` accepts a site name as well as a site ID, since newer
  controller UIs do not always expose the ID.
* Both resources support `terraform import`, as `"<site>:<id>"` where `<site>`
  is either the site name or its ID.

NOTES:

* Tested against controller 6.3.0.45 with an ER605 v2 gateway. Other versions
  are likely to work but are unverified; the controller's API differs between
  versions in ways this provider has already had to account for.
* `omada_vlan` binds a new VLAN to the gateway's LAN ports, taken from the
  site's primary LAN network. Selecting a subset of ports per VLAN is not yet
  supported.
