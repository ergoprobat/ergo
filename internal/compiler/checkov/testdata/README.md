# Captured Checkov output

The JSON files in `captured/` are real `checkov` 3.2.495 output. The Terraform and Dockerfile inputs live in `fixtures/` with a `.fixture` suffix so this repository's scanners do not treat a deliberate violation as source. Copy a fixture to a `.tf` or `Dockerfile` name only while capturing.

`checkov -o json --quiet` omits `passed_checks`. These captures do not use `--quiet`.

```bash
python3 -m venv /tmp/ergo-checkov
/tmp/ergo-checkov/bin/pip install 'checkov==3.2.495'
chk=/tmp/ergo-checkov/bin/checkov
work=$(mktemp -d)
cp fixtures/pass.tf.fixture "$work/pass.tf"
cp fixtures/fail.tf.fixture "$work/fail.tf"
cp fixtures/skip.tf.fixture "$work/skip.tf"
cp fixtures/broken.tf.fixture "$work/broken.tf"
$chk -f "$work/pass.tf" --check CKV_AWS_24 -o json > captured/pass.json
$chk -f "$work/fail.tf" --check CKV_AWS_24 -o json > captured/fail.json
$chk -f "$work/skip.tf" --check CKV_AWS_24 -o json > captured/skip.json
$chk -f "$work/broken.tf" -o json > captured/parse-error.json
empty=$(mktemp -d)
$chk -d "$empty" -o json > captured/empty.json
clean=$(mktemp -d)
cp fixtures/pass.tf.fixture "$clean/main.tf"
cp fixtures/Dockerfile.fixture "$clean/Dockerfile"
$chk -d "$clean" --framework terraform dockerfile -o json > captured/multi.json
```
