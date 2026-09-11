data "omada_ssid" "iot" {
  wlan_group_id = data.omada_wlan_group.default.id
  name          = "Home-IoT"
}
