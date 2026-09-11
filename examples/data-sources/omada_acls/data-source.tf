data "omada_acls" "all" {}

output "acls" {
  value = data.omada_acls.all.acls
}
