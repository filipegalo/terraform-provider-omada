data "omada_ssids" "all" {
  wlan_group_id = data.omada_wlan_group.default.id
}

output "ssids" {
  value = data.omada_ssids.all.ssids
}
