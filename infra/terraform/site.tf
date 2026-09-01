# Permanent public host for the official APK download site.
#
# The site domain is intentionally separated from the Control Plane address.
# It is advertised and therefore expected to be blocked or receive download
# bursts first; neither event may share an address with onboarding or config.

variable "site_enabled" {
  description = "Keep the approved permanent APK site host in production."
  type        = bool
  default     = true
}

variable "site_size" {
  description = "Approved size for site-1. Changing it needs Business Owner approval."
  type        = string
  default     = "s-1vcpu-512mb-10gb"

  validation {
    condition     = var.site_size == "s-1vcpu-512mb-10gb"
    error_message = "site-1 is approved only at 1 vCPU / 512 MB / 10 GB. Record new Business Owner approval before changing it."
  }
}

data "digitalocean_ssh_key" "site" {
  for_each = var.site_enabled ? toset(var.ssh_key_names) : toset([])
  name     = each.value
}

resource "digitalocean_droplet" "site" {
  count = var.site_enabled ? 1 : 0

  name       = "site-1"
  region     = "ams3"
  size       = var.site_size
  image      = "ubuntu-24-04-x64"
  monitoring = true
  ipv6       = true
  ssh_keys   = [for key in data.digitalocean_ssh_key.site : key.id]

  user_data = templatefile("${path.module}/cloud-init/site.yaml.tftpl", {
    index_html_b64    = base64encode(file("${path.module}/../../sites/official/index.html"))
    notfound_html_b64 = base64encode(file("${path.module}/../../sites/official/404.html"))
    styles_css_b64    = base64encode(file("${path.module}/../../sites/official/styles.css"))
    app_js_b64        = base64encode(file("${path.module}/../../sites/official/app.js"))
    nginx_conf_b64    = base64encode(file("${path.module}/../../sites/official/nginx.conf"))
  })

  tags = ["simple-vpn", "site", "permanent"]

  lifecycle {
    prevent_destroy = true

    # DigitalOcean does not return the original cloud-init payload when an
    # existing Droplet is imported. Treating that unreadable value as drift
    # would request a replacement of the permanent site host. Site content is
    # maintained by the host bootstrap/recovery path, not by replacing it.
    ignore_changes = [user_data]
  }
}

resource "digitalocean_firewall" "site" {
  count = var.site_enabled ? 1 : 0

  name        = "site-1-public-web-only"
  droplet_ids = [digitalocean_droplet.site[0].id]

  inbound_rule {
    protocol         = "tcp"
    port_range       = "80"
    source_addresses = ["0.0.0.0/0", "::/0"]
  }

  inbound_rule {
    protocol         = "tcp"
    port_range       = "443"
    source_addresses = ["0.0.0.0/0", "::/0"]
  }

  outbound_rule {
    protocol              = "tcp"
    port_range            = "all"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }

  outbound_rule {
    protocol              = "udp"
    port_range            = "all"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }

  outbound_rule {
    protocol              = "icmp"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }
}

resource "digitalocean_project_resources" "site" {
  count = var.site_enabled ? 1 : 0

  project   = data.digitalocean_project.main.id
  resources = [digitalocean_droplet.site[0].urn]
}

output "site_ip" {
  description = "Public address of site-1. Kept out of public workflow logs."
  value       = var.site_enabled ? digitalocean_droplet.site[0].ipv4_address : ""
  sensitive   = true
}

output "site_monthly_usd" {
  description = "Approved recurring price of site-1."
  value       = var.site_enabled ? 4 : 0
}
