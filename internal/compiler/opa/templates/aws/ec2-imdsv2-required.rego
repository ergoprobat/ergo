# METADATA
# title: EC2 instances require IMDSv2
# description: Instances and launch templates set http_tokens to required, or disable the metadata endpoint.
# custom:
#   provider: aws
#   resources:
#     - aws_instance
#     - aws_launch_template
package ipso.templates.aws.ec2_imdsv2_required

import data.ipso.lib.terraform as tf

kinds := {"aws_instance", "aws_launch_template"}

deny contains tf.violation(rc, msg) if {
	some kind in kinds
	some rc in tf.resources(kind)
	not imds_ok(rc)
	msg := imds_msg(rc)
}

imds_ok(rc) if {
	some i, opt in object.get(rc.change.after, "metadata_options", [])
	is_object(opt)
	tf.equals(rc, ["metadata_options", i, "http_endpoint"], "disabled")
}

imds_ok(rc) if {
	some i, opt in object.get(rc.change.after, "metadata_options", [])
	is_object(opt)
	tf.equals(rc, ["metadata_options", i, "http_tokens"], "required")
}

imds_msg(rc) := sprintf("%s http_tokens is unknown at plan time", [rc.address]) if {
	tf.unknown(rc, ["metadata_options"])
} else := sprintf("%s http_tokens is unknown at plan time", [rc.address]) if {
	some i, opt in object.get(rc.change.after, "metadata_options", [])
	is_object(opt)
	tf.unknown(rc, ["metadata_options", i, "http_tokens"])
} else := sprintf("%s http_tokens must be required", [rc.address])
