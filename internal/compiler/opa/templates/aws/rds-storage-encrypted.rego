# METADATA
# title: RDS storage is encrypted
# description: RDS instances and clusters set storage_encrypted.
# custom:
#   provider: aws
#   resources:
#     - aws_db_instance
#     - aws_rds_cluster
package ipso.templates.aws.rds_storage_encrypted

import data.ipso.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_db_instance")
	msg := tf.require_msg(rc, ["storage_encrypted"], true, "storage_encrypted")
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_rds_cluster")
	msg := tf.require_msg(rc, ["storage_encrypted"], true, "storage_encrypted")
}
