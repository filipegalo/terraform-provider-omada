data "omada_wlan_groups" "all" {}

output "wlan_groups" {
  value = data.omada_wlan_groups.all.groups
}
