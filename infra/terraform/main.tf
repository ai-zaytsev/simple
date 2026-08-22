# Fleet definition.
#
# Cost is bounded by construction, not by convention: sizes and counts are
# variables with validation, and the monthly estimate is an output that fails
# loudly if it drifts above the approved figure.
#
# Droplets are not created in this commit. Resources land once the SSH key
# inventory is confirmed, so that no droplet is ever reachable by password.

data "digitalocean_project" "main" {
  name = var.project_name
}

locals {
  # Monthly USD prices for the sizes used here. Sourced from the DigitalOcean
  # pricing page, checked 2026-08-21. If a size changes, this map changes with
  # it, and the budget guard below reacts.
  size_price = {
    "s-1vcpu-512mb-10gb" = 4
    "s-1vcpu-1gb"        = 6
    "s-1vcpu-2gb"        = 12
  }

  spaces_monthly = 5

  droplet_monthly = (
    lookup(local.size_price, var.obs_size, 0) +
    lookup(local.size_price, var.node_size, 0) * var.vpn_node_count +
    lookup(local.size_price, var.node_size, 0) * var.edge_node_count
  )

  # The ephemeral verification node counts too. It lives for minutes, but the
  # budget guard must see it: an exception that skips the guard is how limits
  # stop meaning anything.
  test_node_monthly = var.test_node_enabled ? lookup(local.size_price, var.node_size, 0) : 0

  estimated_monthly = local.droplet_monthly + local.spaces_monthly + local.test_node_monthly

  # Hard limit set by Business Owner. Anything above needs approval, so it is
  # a failed plan rather than a surprise on the invoice.
  budget_limit = 45
}

check "budget" {
  assert {
    condition     = local.estimated_monthly <= local.budget_limit
    error_message = "Estimated monthly cost ${local.estimated_monthly} USD exceeds the ${local.budget_limit} USD limit. Business Owner approval is required before raising counts or sizes."
  }
}
