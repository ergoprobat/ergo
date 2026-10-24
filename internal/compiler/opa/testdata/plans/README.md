# Captured plans

`show-terraform.json` and `show-tofu.json` are the output of `terraform show -json` and `tofu show -json` for `main.tf`. Terraform 1.16.4 and OpenTofu 1.13.2 produced them on 2026-10-24 against hashicorp/aws 6.x. The provider is configured with static test keys and never contacts AWS.

Regenerate both files from this directory:

```sh
./capture.sh
```

The script needs `terraform` and `tofu` on `PATH` and network access for `init`. Tests read the committed files and do not run either tool.
