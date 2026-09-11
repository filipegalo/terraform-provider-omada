data "omada_site" "current" {}

output "site_id" {
  value = data.omada_site.current.id
}
