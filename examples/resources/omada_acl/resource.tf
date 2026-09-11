resource "omada_acl" "home_to_iot_https" {
  type             = "gateway"
  name             = "Home to IoT HTTPS"
  policy           = "permit"
  protocols        = [6] # TCP
  source_type      = 0
  source_ids       = [omada_vlan.home.id]
  destination_type = 0
  destination_ids  = [omada_vlan.iot.id]
  lan_to_lan       = true
  lan_to_wan       = false
}
