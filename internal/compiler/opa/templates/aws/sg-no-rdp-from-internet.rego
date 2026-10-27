# METADATA
# title: Security groups do not allow RDP from the internet
# description: No ingress on port 3389 from 0.0.0.0/0 or ::/0, including security group rules and VPC ingress rules.
# custom:
#   provider: aws
#   resources:
#     - aws_security_group
#     - aws_security_group_rule
#     - aws_vpc_security_group_ingress_rule
package ipso.templates.aws.sg_no_rdp_from_internet

import data.ipso.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_security_group")
	tf.unknown(rc, ["ingress"])
	msg := sprintf("%s ingress is unknown at plan time", [rc.address])
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_security_group")
	not tf.unknown(rc, ["ingress"])
	some i, rule in object.get(rc.change.after, "ingress", [])
	is_object(rule)
	tf.aws_exposed(rc, ["ingress", i], 3389)
	msg := tf.aws_expose_msg(rc, ["ingress", i], "RDP")
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_security_group_rule")
	ingress_rule(rc)
	tf.aws_exposed(rc, [], 3389)
	msg := tf.aws_expose_msg(rc, [], "RDP")
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_vpc_security_group_ingress_rule")
	tf.aws_exposed(rc, [], 3389)
	msg := tf.aws_expose_msg(rc, [], "RDP")
}

ingress_rule(rc) if tf.equals(rc, ["type"], "ingress")

ingress_rule(rc) if tf.unknown(rc, ["type"])
