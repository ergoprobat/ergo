# METADATA
# title: S3 buckets block public access
# description: Every S3 bucket has a public access block with all four settings true.
# custom:
#   provider: aws
#   resources:
#     - aws_s3_bucket
#     - aws_s3_bucket_public_access_block
package ipso.templates.aws.s3_public_access_block

import data.ipso.lib.terraform as tf

pab_attrs := ["block_public_acls", "block_public_policy", "ignore_public_acls", "restrict_public_buckets"]

deny contains tf.violation(bucket, msg) if {
	some bucket in tf.resources("aws_s3_bucket")
	not good(bucket)
	msg := problem(bucket)
}

good(bucket) if {
	some pab in tf.resources("aws_s3_bucket_public_access_block")
	tf.same_bucket(pab, bucket)
	every attr in pab_attrs {
		tf.equals(pab, [attr], true)
	}
}

problem(bucket) := sprintf("%s %s is unknown at plan time", [bucket.address, concat(", ", names)]) if {
	names := [attr |
		some pab in linked(bucket)
		some attr in unknown_attrs(pab)
	]
	count(names) > 0
} else := sprintf("%s %s must be true", [bucket.address, concat(", ", names)]) if {
	names := [attr |
		some pab in linked(bucket)
		some attr in bad_attrs(pab)
	]
	count(names) > 0
} else := sprintf("%s has no public access block", [bucket.address])

linked(bucket) := [pab |
	some pab in tf.resources("aws_s3_bucket_public_access_block")
	tf.same_bucket(pab, bucket)
]

unknown_attrs(pab) := [attr |
	some attr in pab_attrs
	tf.unknown(pab, [attr])
]

bad_attrs(pab) := [attr |
	some attr in pab_attrs
	not tf.equals(pab, [attr], true)
]
