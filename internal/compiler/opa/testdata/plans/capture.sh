#!/bin/sh
set -eu
cd "$(dirname "$0")"
rm -rf .terraform .terraform.lock.hcl
for tool in terraform tofu; do
  "$tool" init -input=false >/dev/null
  "$tool" plan -input=false -out=plan.bin >/dev/null
  "$tool" show -json plan.bin > "show-$tool.json"
  rm -f plan.bin
  rm -rf .terraform .terraform.lock.hcl
done
