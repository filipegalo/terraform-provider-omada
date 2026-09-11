resource "omada_switch_port_profile" "ap_trunk" {
  name                 = "AP-Trunk"
  native_network_id    = omada_vlan.management.id
  tagged_network_ids   = [omada_vlan.home.id, omada_vlan.iot.id, omada_vlan.guest.id]
  vlan_config_enable   = true
  network_tags_setting = 2 # Custom
  poe                  = 2 # Follow the switch PoE defaults
  lldp_med             = true
  spanning_tree        = true
  stp_edge_port        = true
}
