# METADATA
# title: S3 bucket ACLs are not public
# description: No S3 bucket ACL uses the canned values public-read or public-read-write. A bucket with no ACL resource is private.
# custom:
#   provider: aws
#   resources:
#     - aws_s3_bucket
#     - aws_s3_bucket_acl
package ipso.templates.aws.s3_no_public_acl

import data.ipso.lib.terraform as tf

deny contains tf.violation(bucket, msg) if {
	some bucket in tf.resources("aws_s3_bucket")
	some acl in tf.resources("aws_s3_bucket_acl")
	tf.same_bucket(acl, bucket)
	msg := tf.forbid_msg(acl, ["acl"], "public-read", "acl")
}

deny contains tf.violation(bucket, msg) if {
	some bucket in tf.resources("aws_s3_bucket")
	some acl in tf.resources("aws_s3_bucket_acl")
	tf.same_bucket(acl, bucket)
	not tf.unknown(acl, ["acl"])
	msg := tf.forbid_msg(acl, ["acl"], "public-read-write", "acl")
}
