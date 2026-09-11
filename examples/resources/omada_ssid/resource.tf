data "omada_wlan_group" "default" {
  name = "Default"
}

resource "omada_ssid" "iot" {
  wlan_group_id = data.omada_wlan_group.default.id
  name          = "Home-IoT"
  band          = 1 # 2.4 GHz
  security      = 3 # WPA-PSK
  psk           = var.iot_wifi_password
  vlan_enable   = true
  vlan_id       = 20
  pmf_mode      = 3
}
