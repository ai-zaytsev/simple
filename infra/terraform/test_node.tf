# Ephemeral verification node.
#
# Not part of the fleet. It exists to answer one question: does a VLESS +
# REALITY endpoint built by our own automation actually carry traffic. It is
# created, used and destroyed inside a single workflow run, so the credential
# it carries never outlives the run and never reaches a log or a repository.
#
# Disabled by default. The normal plan creates nothing.

variable "test_node_enabled" {
  description = "Create the ephemeral verification node. Off unless a run explicitly asks for it."
  type        = bool
  default     = false
}

variable "test_node_uuid" {
  description = "VLESS credential for the verification node. Generated per run, never stored."
  type        = string
  default     = ""
  sensitive   = true
}

variable "test_node_reality_private_key" {
  description = "REALITY private key for the verification node. Generated per run, never stored."
  type        = string
  default     = ""
  sensitive   = true
}

variable "test_node_reality_short_id" {
  description = "REALITY short id for the verification node."
  type        = string
  default     = ""
  sensitive   = true
}

variable "test_node_dest" {
  description = "Real site the node proxies to when authentication fails. This is what a scanner sees."
  type        = string
  default     = "www.microsoft.com:443"
}

variable "test_node_server_name" {
  description = "SNI the client presents, matching the dest site."
  type        = string
  default     = "www.microsoft.com"
}

variable "xray_version" {
  description = "Xray-core release installed on the node. Pinned: an unpinned install makes the test unrepeatable."
  type        = string
  default     = "v25.8.3"
}

data "digitalocean_ssh_key" "fleet" {
  for_each = var.test_node_enabled ? toset(var.ssh_key_names) : toset([])
  name     = each.value
}

resource "digitalocean_droplet" "test_node" {
  count = var.test_node_enabled ? 1 : 0

  name     = "test-node-vless"
  region   = var.region
  size     = var.node_size
  image    = "ubuntu-24-04-x64"
  ssh_keys = [for k in data.digitalocean_ssh_key.fleet : k.id]

  # Everything the node needs arrives here. No manual SSH step exists, which is
  # the same rule the real fleet follows: see docs/architecture/node-lifecycle.md.
  user_data = templatefile("${path.module}/cloud-init/test-node.yaml.tftpl", {
    xray_version = var.xray_version
    uuid         = var.test_node_uuid
    private_key  = var.test_node_reality_private_key
    short_id     = var.test_node_reality_short_id
    dest         = var.test_node_dest
    server_name  = var.test_node_server_name
  })

  tags = ["simple-vpn", "ephemeral", "verification"]

  lifecycle {
    precondition {
      condition = !var.test_node_enabled || (
        var.test_node_uuid != "" &&
        var.test_node_reality_private_key != "" &&
        var.test_node_reality_short_id != ""
      )
      error_message = "The verification node needs a generated credential. Creating one without it would produce an endpoint nobody can reach."
    }
  }
}

resource "digitalocean_project_resources" "test_node" {
  count     = var.test_node_enabled ? 1 : 0
  project   = data.digitalocean_project.main.id
  resources = [digitalocean_droplet.test_node[0].urn]
}

output "test_node_ip" {
  description = "Public address of the verification node, empty when it does not exist."
  value       = var.test_node_enabled ? digitalocean_droplet.test_node[0].ipv4_address : ""
}
