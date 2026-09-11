# Changelog

## 0.2.0 (2026-09-11)

FEATURES:

* **New Resource:** `omada_switch_port` — manages the configuration of an
  existing physical switch port, including its name, port profile, VLAN
  overrides, link speed and duplex settings. Removing it from Terraform only
  relinquishes management and does not reset the live port.
* **New Resource:** `omada_switch_port_profile` — manages reusable switch port
  profiles and their native, tagged and untagged VLAN associations. Updates
  preserve controller-owned and unmodelled PoE, STP and LLDP settings.
* **New Data Source:** `omada_site` — resolves a site by name or ID, or returns
  the provider's configured site.
* **New Data Source:** `omada_vlan` — resolves an existing VLAN by name,
  internal network ID or IEEE 802.1Q VLAN ID.
* **New Data Source:** `omada_switch_port_profile` — resolves an existing port
  profile by name or ID without taking ownership of it.
* **New Data Source:** `omada_switch` — resolves an adopted switch by name or
  MAC and exposes its model, status, firmware information and physical ports.

NOTES:

* Switch port, profile and all four data-source reads were verified against a
  controller running 6.3.0.45. Port-profile writes use read-modify-write so
  fields outside Terraform's schema survive an update.

## 0.1.0 (2026-09-11)

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
