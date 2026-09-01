#!/usr/bin/env python3
"""Keep the address hand-off between APK site workflow steps explicit."""

from pathlib import Path

import yaml


workflow = yaml.safe_load(Path(".github/workflows/deploy-apk-site.yml").read_text())
step_list = workflow["jobs"]["deploy"]["steps"]
steps = {step.get("name"): step for step in step_list}
step_names = [step.get("name") for step in step_list]

apply_script = steps["Apply"]["run"]
recovery_script = steps["Recover the existing origin when direct health fails"]["run"]
inventory_script = steps["Confirm live inventory"]["run"]
inventory_env = steps["Confirm live inventory"]["env"]
reconcile_script = steps["Reconcile exact existing site resources into Terraform state"]["run"]
content_step = steps["Deploy the repository-owned static site content"]
fingerprint_step = steps["Show deployment key fingerprints"]
bootstrap_step = steps["Bootstrap CI access to the existing site"]

assert fingerprint_step["env"]["DEPLOY_KEY"] == "${{ secrets.CP_DEPLOY_SSH_KEY }}"
assert "/v2/account/keys?per_page=200" in fingerprint_step["run"]
assert "simple-vpn-ssh-key" in fingerprint_step["run"]
assert "ssh-keygen -E md5 -lf" in fingerprint_step["run"]
assert bootstrap_step["run"] == "bash .github/scripts/bootstrap-apk-site-access.sh"
assert bootstrap_step["env"]["BOOTSTRAP_CIDR"] == "${{ inputs.bootstrap_cidr }}"
assert bootstrap_step["env"]["DEPLOY_KEY"] == "${{ secrets.CP_DEPLOY_SSH_KEY }}"
assert step_names.index("Wait for cloud-init and HTTPS") < step_names.index(
    "Bootstrap CI access to the existing site"
) < step_names.index("Deploy the repository-owned static site content")

assert "terraform apply -input=false -auto-approve /tmp/site.tfplan" in apply_script
assert "ip=$(terraform output -raw site_ip)" in apply_script
assert 'echo "SITE_IP=${ip}" >> "${GITHUB_ENV}"' in apply_script

assert 'ip="${SITE_IP:-}"' in recovery_script
assert "terraform output" not in recovery_script
assert "power_cycle" in recovery_script

assert "terraform state list > /tmp/site-state-list.txt" in inventory_script
assert "terraform state list > /tmp/site-state-list.txt" in reconcile_script
assert "state list 2>/dev/null | grep" not in inventory_script
assert "state list | grep" not in reconcile_script
assert inventory_env["AWS_ACCESS_KEY_ID"] == "${{ secrets.SPACES_ACCESS_KEY_ID }}"
assert inventory_env["AWS_SECRET_ACCESS_KEY"] == "${{ secrets.SPACES_SECRET_ACCESS_KEY }}"

assert content_step["run"] == "bash .github/scripts/deploy-apk-site-content.sh"
assert content_step["env"]["SITE_DROPLET_ID"] == "${{ steps.inventory.outputs.droplet_id }}"
assert content_step["env"]["DEPLOY_KEY"] == "${{ secrets.CP_DEPLOY_SSH_KEY }}"
assert step_names.index("Wait for cloud-init and HTTPS") < step_names.index(
    "Deploy the repository-owned static site content"
) < step_names.index("Put Cloudflare in front")

content_script = Path(".github/scripts/deploy-apk-site-content.sh").read_text()
assert 'runner_cidr="${runner_ip}/32"' in content_script
assert "trap finish EXIT" in content_script
assert "firewall-original.json" in content_script
assert "firewall_open=true" in content_script
assert "Temporary SSH access was removed" in content_script
assert "0.0.0.0/0" not in content_script
assert content_script.index("firewall_open=true") < content_script.index("open-answer.json")
assert "origin-index.html" in content_script
assert "| grep -q 'data-install-guide'" not in content_script

bootstrap_script = Path(".github/scripts/bootstrap-apk-site-access.sh").read_text()
assert 'BOOTSTRAP_CIDR' in bootstrap_script
assert '/32' in bootstrap_script
assert "trap finish EXIT" in bootstrap_script
assert "firewall-original.json" in bootstrap_script
assert "firewall_open=true" in bootstrap_script
assert "Bootstrap SSH access was removed" in bootstrap_script
assert "0.0.0.0/0" not in bootstrap_script
assert "site-1 now accepts the CI deployment key" in bootstrap_script

print("ok: remote state has credentials, avoids pipefail/SIGPIPE, and SITE_IP crosses steps")
