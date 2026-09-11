# Manages the configuration of an existing physical port. It does not create
# or delete a port: removing this resource only stops Terraform management.
resource "omada_switch_port" "camera_entrance" {
  switch_mac = "D8-44-89-38-C6-C0"
  port       = 2

  name              = "CAMERA MAIN FLOOR - ENTRANCE"
  profile_id        = "6a9ef808c7e1e27192728693"
  native_network_id = "6a9ef808c7e1e2719272868b"

  # 0 = Allow All, 1 = Block All. Custom VLAN membership is configured by
  # assigning a profile_id; its value can still be observed during refresh.
  network_tags_setting = 1
  link_speed           = 0 # Auto
  duplex               = 0 # Auto
}
