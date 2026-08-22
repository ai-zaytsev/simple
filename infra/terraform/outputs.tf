output "estimated_monthly_usd" {
  description = "Estimated DigitalOcean spend per month, droplets plus Spaces."
  value       = local.estimated_monthly
}

output "budget_headroom_usd" {
  description = "Room left under the approved limit."
  value       = local.budget_limit - local.estimated_monthly
}

output "project_id" {
  description = "Project every resource is assigned to."
  value       = data.digitalocean_project.main.id
}
