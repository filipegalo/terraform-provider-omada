# Switches SNMP off for the provider's site. The community string and the
# SNMPv3 credentials stay in the controller and are not managed.
resource "omada_site_snmp" "this" {
  v1v2c_enabled = false
  v3_enabled    = false
}
