variable "project_name" {
  description = "DigitalOcean project that owns every resource created here."
  type        = string
  default     = "simple-vpn-prod"
}

variable "region" {
  description = "DigitalOcean region slug for the disposable fleet."
  type        = string
  default     = "ams3"
}

variable "ssh_key_names" {
  description = "Names of SSH keys already registered in DigitalOcean. Droplets get no password login."
  type        = list(string)
  default     = ["simple-vpn-ssh-key"]

  validation {
    condition     = length(var.ssh_key_names) > 0
    error_message = "At least one SSH key is required. A droplet created without a key gets a root password by email, which puts a secret in a mailbox and a password login on a public port."
  }
}

variable "obs_size" {
  description = "Droplet size for the observability host."
  type        = string
  default     = "s-1vcpu-1gb"
}

variable "node_size" {
  description = "Droplet size for VPN and edge nodes."
  type        = string
  default     = "s-1vcpu-512mb-10gb"
}

variable "vpn_node_count" {
  description = "Number of VPN nodes. Three is the minimum for primary plus two reserves."
  type        = number
  default     = 3

  validation {
    condition     = var.vpn_node_count >= 3 && var.vpn_node_count <= 6
    error_message = "Below three the connection plan cannot be filled. Above six the budget is at risk: raise the cap only with Business Owner approval."
  }
}

variable "edge_node_count" {
  description = "Number of bootstrap edge nodes."
  type        = number
  default     = 2

  validation {
    condition     = var.edge_node_count >= 2 && var.edge_node_count <= 4
    error_message = "Fewer than two entry points is a single point of entry. More than four is outside the approved budget."
  }
}
