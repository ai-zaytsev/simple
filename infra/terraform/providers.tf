provider "digitalocean" {
  # Token comes from DIGITALOCEAN_TOKEN in the environment.
  # The token is deliberately narrow: it can read projects, droplets, keys,
  # regions and sizes, and it can write. It cannot read the account.
  # See docs/integrations/digitalocean.md.
}
