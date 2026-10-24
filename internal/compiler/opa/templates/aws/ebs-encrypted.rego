# METADATA
# title: EBS volumes are encrypted
# description: EBS volumes, instance root and extra volumes, and launch template EBS mappings have encrypted set.
# custom:
#   provider: aws
#   resources:
#     - aws_ebs_volume
#     - aws_instance
#     - aws_launch_template
package ergo.templates.aws.ebs_encrypted

import data.ergo.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_ebs_volume")
	msg := tf.require_msg(rc, ["encrypted"], true, "encrypted")
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_instance")
	tf.unknown(rc, ["root_block_device"])
	msg := sprintf("%s root_block_device is unknown at plan time", [rc.address])
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_instance")
	not tf.unknown(rc, ["root_block_device"])
	count(object.get(rc.change.after, "root_block_device", [])) == 0
	msg := sprintf("%s root_block_device.encrypted must be true", [rc.address])
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_instance")
	some i, disk in object.get(rc.change.after, "root_block_device", [])
	is_object(disk)
	msg := tf.require_msg(rc, ["root_block_device", i, "encrypted"], true, "encrypted")
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_instance")
	tf.unknown(rc, ["ebs_block_device"])
	msg := sprintf("%s ebs_block_device is unknown at plan time", [rc.address])
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_instance")
	some i, disk in object.get(rc.change.after, "ebs_block_device", [])
	is_object(disk)
	msg := tf.require_msg(rc, ["ebs_block_device", i, "encrypted"], true, "encrypted")
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_launch_template")
	tf.unknown(rc, ["block_device_mappings"])
	msg := sprintf("%s block_device_mappings is unknown at plan time", [rc.address])
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_launch_template")
	some i, mapping in object.get(rc.change.after, "block_device_mappings", [])
	is_object(mapping)
	tf.unknown(rc, ["block_device_mappings", i, "ebs"])
	msg := sprintf("%s encrypted is unknown at plan time", [rc.address])
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_launch_template")
	some i, mapping in object.get(rc.change.after, "block_device_mappings", [])
	is_object(mapping)
	some j, disk in object.get(mapping, "ebs", [])
	is_object(disk)
	msg := tf.require_msg(rc, ["block_device_mappings", i, "ebs", j, "encrypted"], true, "encrypted")
}
