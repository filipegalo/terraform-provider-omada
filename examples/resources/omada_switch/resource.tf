# Manages the name of a switch the controller has already adopted. It does not
# adopt or forget a switch: removing this resource only stops Terraform
# management. Regular and Agile Series (Easy Managed) switches both work.
resource "omada_switch" "core" {
  mac  = "AA-BB-CC-DD-EE-FF"
  name = "core-switch"
}
