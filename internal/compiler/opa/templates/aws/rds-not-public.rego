# METADATA
# title: RDS instances are not publicly accessible
# description: RDS instances do not set publicly_accessible. A missing value keeps the provider default, which is not public.
# custom:
#   provider: aws
#   resources:
#     - aws_db_instance
package ipso.templates.aws.rds_not_public

import data.ipso.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_db_instance")
	msg := tf.forbid_msg(rc, ["publicly_accessible"], true, "publicly_accessible")
}
