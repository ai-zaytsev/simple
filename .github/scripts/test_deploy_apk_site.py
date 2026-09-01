#!/usr/bin/env python3
"""Keep the address hand-off between APK site workflow steps explicit."""

from pathlib import Path

import yaml


workflow = yaml.safe_load(Path(".github/workflows/deploy-apk-site.yml").read_text())
steps = {step.get("name"): step for step in workflow["jobs"]["deploy"]["steps"]}

apply_script = steps["Apply"]["run"]
recovery_script = steps["Recover the existing origin when direct health fails"]["run"]
inventory_script = steps["Confirm live inventory"]["run"]
inventory_env = steps["Confirm live inventory"]["env"]
reconcile_script = steps["Reconcile exact existing site resources into Terraform state"]["run"]

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

print("ok: remote state has credentials, avoids pipefail/SIGPIPE, and SITE_IP crosses steps")
