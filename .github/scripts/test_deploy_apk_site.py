#!/usr/bin/env python3
"""Keep the address hand-off between APK site workflow steps explicit."""

from pathlib import Path

import yaml


workflow = yaml.safe_load(Path(".github/workflows/deploy-apk-site.yml").read_text())
steps = {step.get("name"): step for step in workflow["jobs"]["deploy"]["steps"]}

apply_script = steps["Apply"]["run"]
recovery_script = steps["Recover the existing origin when direct health fails"]["run"]

assert "terraform apply -input=false -auto-approve /tmp/site.tfplan" in apply_script
assert "ip=$(terraform output -raw site_ip)" in apply_script
assert 'echo "SITE_IP=${ip}" >> "${GITHUB_ENV}"' in apply_script

assert 'ip="${SITE_IP:-}"' in recovery_script
assert "terraform output" not in recovery_script
assert "power_cycle" in recovery_script

print("ok: Apply produces SITE_IP and Recovery consumes it without reopening state")
