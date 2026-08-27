# Ephemeral verification node.
#
# Not part of the fleet. It exists to answer one question: does a node built by
# our own automation actually carry traffic. It is created, used and destroyed
# inside a single workflow run, so its credentials never outlive the run.
#
# Certificates come from the Let's Encrypt staging environment. Production has
# a hard limit of five certificates per identical name set per week, and a
# create-verify-destroy loop on the production endpoint would exhaust a
# domain's weekly budget in one afternoon. See ADR-023.
#
# Disabled by default. The normal plan creates nothing.

variable "test_node_enabled" {
  description = "Create the ephemeral verification node. Off unless a run explicitly asks for it."
  type        = bool
  default     = false
}

variable "test_node_domain" {
  description = "Entry domain the verification node serves and obtains a certificate for."
  type        = string
  default     = ""
}

variable "cover_site" {
  description = "Which cover site this node serves. Two nodes with the same one are one group to anybody comparing hashes, so each gets its own."
  type        = string
  default     = "pigeons"

  validation {
    condition     = contains(["pigeons", "trams", "sourdough"], var.cover_site)
    error_message = "cover_site must name a directory under sites/."
  }
}

variable "test_node_ws_uuid" {
  description = "VLESS credential for the WebSocket transport. Generated per run, never stored."
  type        = string
  default     = ""
  sensitive   = true
}

variable "test_node_ws_path" {
  description = "Dedicated path the tunnel lives on. Everything else on the domain is the site."
  type        = string
  default     = ""
  sensitive   = true
}

variable "test_node_reality_uuid" {
  description = "VLESS credential for the standby REALITY transport."
  type        = string
  default     = ""
  sensitive   = true
}

variable "test_node_reality_private_key" {
  description = "REALITY private key. Generated per run, never stored."
  type        = string
  default     = ""
  sensitive   = true
}

variable "test_node_reality_short_id" {
  description = "REALITY short id."
  type        = string
  default     = ""
  sensitive   = true
}

variable "acme_email" {
  description = "Contact address for the ACME account."
  type        = string
  default     = ""
}

variable "acme_staging" {
  description = "Use the Let's Encrypt staging environment. True unless a run explicitly asks to spend a real certificate."
  type        = bool
  default     = true
}

variable "acme_production_ack" {
  description = "Explicit acknowledgement that this run may spend one real certificate against the domain's weekly budget."
  type        = bool
  default     = false
}

variable "debug_status" {
  description = "Publish bring-up progress at /status.txt. Verification nodes only: in production it would describe our own bring-up to anyone asking."
  type        = bool
  default     = false
}

variable "reality_port" {
  description = "Port for the standby transport. Kept off 443, which the site and the tunnel already use."
  type        = number
  default     = 8443
}

variable "reality_dest" {
  description = "Real site the standby transport proxies to when authentication fails."
  type        = string
  default     = "www.microsoft.com:443"
}

variable "reality_server_name" {
  description = "SNI the standby transport presents, matching its dest."
  type        = string
  default     = "www.microsoft.com"
}

variable "ws_backend_port" {
  description = "Loopback port where Xray listens for WebSocket upgrades from Nginx."
  type        = number
  default     = 10000
}

variable "lego_version" {
  description = "ACME client release. A single static binary, pinned like Xray: an unpinned client makes bring-up unrepeatable."
  type        = string
  default     = "v5.4.0"
}

variable "xray_version" {
  description = "Xray-core release installed on the node. Pinned: an unpinned install makes the run unrepeatable."
  type        = string
  default     = "v25.8.3"
}

data "digitalocean_ssh_key" "fleet" {
  for_each = var.test_node_enabled ? toset(var.ssh_key_names) : toset([])
  name     = each.value
}

resource "digitalocean_droplet" "test_node" {
  count = var.test_node_enabled ? 1 : 0

  name     = "test-node"
  region   = var.region
  size     = var.node_size
  image    = "ubuntu-24-04-x64"
  ssh_keys = [for k in data.digitalocean_ssh_key.fleet : k.id]

  user_data = templatefile("${path.module}/cloud-init/node.yaml.tftpl", {
    domain              = var.test_node_domain
    acme_email          = var.acme_email
    acme_staging        = var.acme_staging ? "true" : "false"
    debug_status        = var.debug_status ? "true" : "false"
    site_html_b64       = base64encode(file("${path.module}/../../sites/${var.cover_site}/index.html"))
    notfound_html_b64   = base64encode(file("${path.module}/../../sites/${var.cover_site}/404.html"))
    ws_path             = var.test_node_ws_path
    ws_uuid             = var.test_node_ws_uuid
    ws_backend_port     = var.ws_backend_port
    reality_port        = var.reality_port
    reality_uuid        = var.test_node_reality_uuid
    reality_private_key = var.test_node_reality_private_key
    reality_short_id    = var.test_node_reality_short_id
    reality_dest        = var.reality_dest
    reality_server_name = var.reality_server_name
    xray_version        = var.xray_version
    lego_version        = var.lego_version
  })

  tags = ["simple-vpn", "ephemeral", "verification"]

  lifecycle {
    precondition {
      condition = !var.test_node_enabled || (
        var.test_node_domain != "" &&
        var.test_node_ws_uuid != "" &&
        var.test_node_ws_path != "" &&
        var.test_node_reality_uuid != "" &&
        var.test_node_reality_private_key != "" &&
        var.test_node_reality_short_id != ""
      )
      error_message = "The verification node needs a domain and generated credentials. Creating one without them produces an endpoint nobody can reach."
    }

    precondition {
      # Staging stays the default, and leaving it takes a deliberate second
      # flag. One switch could be flipped by a run that only meant to test
      # something else; two cannot be flipped by accident. Five certificates
      # per identical name set per week means an automated
      # create-verify-destroy loop on production would exhaust a domain's
      # weekly budget in one afternoon.
      condition     = !var.test_node_enabled || var.acme_staging || var.acme_production_ack
      error_message = "Spending a real certificate needs an explicit acknowledgement. Set acme_production_ack when the run is meant to produce a node a device will actually connect to."
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

# Credentials are published back as outputs so that a later run can reuse them.
#
# Every rebuild of the application used to generate fresh credentials, which
# changed the node's user data, which replaced the droplet, which spent another
# certificate. Five per name per week is a budget that ran out in an afternoon
# of debugging a client that the node had nothing to do with.
#
# Reading these back and passing them in again produces identical user data, so
# the droplet is left alone and its certificate keeps its full lifetime.
#
# They are marked sensitive: Terraform then refuses to print them in a plan or
# an apply, and they only leave through an explicit request for one value.

output "test_node_domain" {
  description = "Domain the existing verification node serves, empty when there is none."
  value       = var.test_node_domain
}

output "test_node_ws_uuid" {
  description = "VLESS credential of the existing node."
  value       = var.test_node_ws_uuid
  sensitive   = true
}

output "test_node_ws_path" {
  description = "Tunnel path of the existing node."
  value       = var.test_node_ws_path
  sensitive   = true
}

output "test_node_reality_uuid" {
  description = "Standby transport credential of the existing node."
  value       = var.test_node_reality_uuid
  sensitive   = true
}

output "test_node_reality_private_key" {
  description = "Standby transport private key of the existing node."
  value       = var.test_node_reality_private_key
  sensitive   = true
}

output "test_node_reality_short_id" {
  description = "Standby transport short id of the existing node."
  value       = var.test_node_reality_short_id
  sensitive   = true
}

output "test_node_acme_staging" {
  description = "Whether the existing node was built against the staging environment."
  value       = var.acme_staging ? "true" : "false"
}
