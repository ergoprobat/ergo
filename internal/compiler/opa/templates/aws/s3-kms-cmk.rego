# METADATA
# title: S3 buckets use a customer-managed KMS key
# description: Every S3 bucket has SSE-KMS with a customer-managed key. alias/aws/s3 is the AWS-managed key.
# custom:
#   provider: aws
#   resources:
#     - aws_s3_bucket
#     - aws_s3_bucket_server_side_encryption_configuration
package ergo.templates.aws.s3_kms_cmk

import data.ergo.lib.terraform as tf

deny contains tf.violation(bucket, msg) if {
	some bucket in tf.resources("aws_s3_bucket")
	not good(bucket)
	msg := problem(bucket)
}

good(bucket) if {
	some enc in tf.resources("aws_s3_bucket_server_side_encryption_configuration")
	tf.same_bucket(enc, bucket)
	kms_cmk(enc)
}

kms_cmk(enc) if {
	some i, rule in object.get(enc.change.after, "rule", [])
	is_object(rule)
	some j, block in object.get(rule, "apply_server_side_encryption_by_default", [])
	is_object(block)
	tf.equals(enc, ["rule", i, "apply_server_side_encryption_by_default", j, "sse_algorithm"], "aws:kms")
	tf.is_set(enc, ["rule", i, "apply_server_side_encryption_by_default", j, "kms_master_key_id"])
	not aws_managed(object.get(block, "kms_master_key_id", ""))
}

aws_managed(value) if value == "alias/aws/s3"

aws_managed(value) if {
	is_string(value)
	endswith(value, ":alias/aws/s3")
}

problem(bucket) := sprintf("%s sse_algorithm is unknown at plan time", [bucket.address]) if {
	some enc in linked(bucket)
	sse_unknown(enc)
} else := sprintf("%s sse_algorithm must be aws:kms", [bucket.address]) if {
	some enc in linked(bucket)
	not sse_is_kms(enc)
} else := sprintf("%s kms_master_key_id must be a customer-managed key", [bucket.address]) if {
	count(linked(bucket)) > 0
} else := sprintf("%s has no SSE-KMS customer-managed key", [bucket.address])

linked(bucket) := [enc |
	some enc in tf.resources("aws_s3_bucket_server_side_encryption_configuration")
	tf.same_bucket(enc, bucket)
]

sse_is_kms(enc) if {
	some i, rule in object.get(enc.change.after, "rule", [])
	is_object(rule)
	some j, block in object.get(rule, "apply_server_side_encryption_by_default", [])
	is_object(block)
	tf.equals(enc, ["rule", i, "apply_server_side_encryption_by_default", j, "sse_algorithm"], "aws:kms")
}

sse_unknown(enc) if tf.unknown(enc, ["rule"])

sse_unknown(enc) if {
	some i, rule in object.get(enc.change.after, "rule", [])
	is_object(rule)
	tf.unknown(enc, ["rule", i, "apply_server_side_encryption_by_default"])
}

sse_unknown(enc) if {
	some i, rule in object.get(enc.change.after, "rule", [])
	is_object(rule)
	some j, block in object.get(rule, "apply_server_side_encryption_by_default", [])
	is_object(block)
	tf.unknown(enc, ["rule", i, "apply_server_side_encryption_by_default", j, "sse_algorithm"])
}
