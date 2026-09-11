data "omada_vlan" "iot" {
  vlan_id = 20
}

output "iot_network_id" {
  value = data.omada_vlan.iot.id
}
