# Changelog

## Unreleased

FEATURES:

* **New Resource:** `omada_switch` — manages the name of a switch the
  controller has already adopted, imported by MAC. Regular and Agile Series
  (Easy Managed) switches are both supported; the provider follows the
  controller's own answer to pick the matching endpoint. Removing it from
  Terraform only relinquishes management and does not forget or reset the
  switch.
* **New Resource:** `omada_site` — manages name, time zone and NTP servers of
  an existing site, imported by site name or ID. It never creates or deletes a
  site. Time zones are the controller's own ids (one IANA name per offset
  group: `Europe/Amsterdam` covers Berlin, `Europe/Berlin` is rejected).
* **New Resource:** `omada_site_snmp` — switches SNMP v1/v2c and v3 of an
  existing site on or off. Credentials are neither read into state nor sent.

NOTES:

* Both resources use the public Open API: the application's access token when
  `client_id` and `client_secret` are configured, the classic session
  otherwise. A switch rename is a partial PATCH that carries only the name. A
  site rename is a full-object read-modify-write, because the endpoint replaces
  the whole site; region, time zone, NTP, DST, scenario and location are sent
  back as read.
* Verified against Omada Controller 6.3.0.45: reads and the endpoint selection
  for Agile Series switches with both authentication modes, writes with Open
  API client credentials.
* Verified against Omada Controller 6.3.0.111 with Open API client
  credentials: import, time zone, NTP and SNMP changes on two sites, a plan
  without changes afterwards. The SNMP PATCH needs both switches in the body.

## 0.4.0 (2026-09-11)

FEATURES:

* provider: optional `client_id` and `client_secret` authenticate public Open API
  requests with an Omada Client-mode application. Credentials can also be read
  from `OMADA_CLIENT_ID` and `OMADA_CLIENT_SECRET`.
* Open API access tokens are refreshed shortly before expiry and once after an
  Omada token-expiry response. Controller-internal web endpoints retain classic
  session authentication even though some of their paths begin with `/openapi`.
  When client credentials are omitted, public Open API paths also fall back to
  classic-session authentication.

## 0.3.0 (2026-09-11)

FEATURES:

* **New Resources:** `omada_wlan_group` and `omada_ssid` manage WLAN groups
  and wireless networks, including band selection, security, VLAN tagging,
  guest mode, 802.11r and PMF. SSID updates preserve the existing write-only
  PSK unless a replacement is explicitly configured.
* **New Resource:** `omada_acl` manages gateway, switch and EAP ACL rules with
  protocol, source, destination and traffic-direction selectors.
* **New Data Sources:** `omada_wlan_group`, `omada_wlan_groups`, `omada_ssid`,
  `omada_ssids`, `omada_acl` and `omada_acls` support detailed lookup and
  inventory/discovery workflows.
* `omada_switch_port_profile` now manages PoE, port isolation, LLDP-MED,
  802.1X, loopback detection, EEE, flow control and spanning-tree settings.

NOTES:

* WLAN, SSID and ACL reads were verified against Omada Controller 6.3.0.45.
  Existing SSIDs and ACLs were imported into a real homelab configuration and
  produced a no-change OpenTofu plan.
* SSID, ACL and port-profile updates use full-object read-modify-write and
  preserve controller-owned fields that are outside the Terraform schema.

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
