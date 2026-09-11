# A gateway-backed VLAN with its own IPv4 gateway and DHCP scope. The VLAN is
# bound to the gateway's LAN ports, taken from the site's primary LAN network.
resource "omada_vlan" "iot" {
  name           = "IoT"
  vlan_id        = 20
  device_mac     = "AA-BB-CC-DD-EE-FF"
  gateway_subnet = "192.0.2.1/24"

  dhcp_range_start = "192.0.2.100"
  dhcp_range_end   = "192.0.2.199"

  # Defaults shown explicitly: dhcp_enabled = true, dhcp_dns_mode = "auto",
  # dhcp_lease_time = 1440, isolation = false.
  dhcp_dns_mode    = "manual"
  dhcp_primary_dns = "192.0.2.53"
  dhcp_lease_time  = 1440
  isolation        = false
}
