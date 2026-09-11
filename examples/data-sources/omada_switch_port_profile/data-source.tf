data "omada_switch_port_profile" "all" {
  name = "All"
}

output "all_profile_id" {
  value = data.omada_switch_port_profile.all.id
}
