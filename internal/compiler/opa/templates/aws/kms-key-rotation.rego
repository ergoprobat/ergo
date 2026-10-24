# METADATA
# title: Symmetric KMS keys rotate
# description: Symmetric customer-managed KMS keys set enable_key_rotation. Asymmetric and HMAC keys are skipped.
# custom:
#   provider: aws
#   resources:
#     - aws_kms_key
package ergo.templates.aws.kms_key_rotation

import data.ergo.lib.terraform as tf

asymmetric_specs := {
	"RSA_2048", "RSA_3072", "RSA_4096",
	"ECC_NIST_P256", "ECC_NIST_P384", "ECC_NIST_P521", "ECC_SECG_P256K1",
	"HMAC_224", "HMAC_256", "HMAC_384", "HMAC_512",
	"SM2",
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_kms_key")
	not asymmetric(rc)
	msg := tf.require_msg(rc, ["enable_key_rotation"], true, "enable_key_rotation")
}

asymmetric(rc) if {
	some spec in asymmetric_specs
	tf.equals(rc, ["customer_master_key_spec"], spec)
}
