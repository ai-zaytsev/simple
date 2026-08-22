# Terraform

Provisioning of the disposable fleet on DigitalOcean.

State lives in the existing Spaces bucket under `terraform/prod.tfstate`. No
additional bucket is created; the bucket itself is configured outside Terraform
because it stores Terraform's own state.

## Running

Credentials are never stored here. Locally:

```
export DIGITALOCEAN_TOKEN=...
export AWS_ACCESS_KEY_ID=...        # SPACES_ACCESS_KEY_ID
export AWS_SECRET_ACCESS_KEY=...    # SPACES_SECRET_ACCESS_KEY
terraform -chdir=infra/terraform init
terraform -chdir=infra/terraform plan
```

In CI the same variables come from GitHub Secrets.

## Budget Guard

`check "budget"` fails the plan when the estimated monthly cost rises above the
approved limit. Raising counts or sizes past that point is a Business Owner
decision, so it surfaces as a failed plan rather than as a larger invoice.

Prices in `local.size_price` are copied from the DigitalOcean pricing page and
must be updated together with any size change.
