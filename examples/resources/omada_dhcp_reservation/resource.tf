resource "omada_dhcp_reservation" "printer" {
  # site_id defaults to the provider's configured site; set it explicitly only
  # to target a different site from this provider instance.
  network_id  = "6142yyyyyyyyyyyyyyyyyyyy"
  mac_address = "AA:BB:CC:DD:EE:FF"
  ip_address  = "192.168.1.50"
  description = "Office printer"
  enabled     = true
}
