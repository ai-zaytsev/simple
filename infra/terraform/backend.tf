# Remote state in the existing Spaces bucket.
#
# Spaces is S3-compatible but is not AWS, so every AWS-specific validation and
# metadata lookup has to be switched off explicitly. Without skip_s3_checksum
# uploads fail against non-AWS stores.
#
# Credentials come from AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY, which are
# fed from SPACES_ACCESS_KEY_ID and SPACES_SECRET_ACCESS_KEY. They are never
# written into this file.

terraform {
  backend "s3" {
    bucket = "simple-vpn-infra-backups"
    key    = "terraform/prod.tfstate"
    region = "us-east-1"

    endpoints = {
      s3 = "https://ams3.digitaloceanspaces.com"
    }

    skip_credentials_validation = true
    skip_region_validation      = true
    skip_metadata_api_check     = true
    skip_requesting_account_id  = true
    skip_s3_checksum            = true
    use_path_style              = false
  }
}
