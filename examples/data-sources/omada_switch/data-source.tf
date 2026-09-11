data "omada_switch" "main" {
  mac = "D8-44-89-38-C6-C0"
}

output "switch_model" {
  value = data.omada_switch.main.model
}

output "switch_ports" {
  value = data.omada_switch.main.ports
}
