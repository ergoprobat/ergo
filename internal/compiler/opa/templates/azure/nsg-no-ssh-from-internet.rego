# METADATA
# title: Network security groups do not allow SSH from the internet
# description: No inbound allow on port 22 from Internet, *, 0.0.0.0/0, or ::/0, including standalone security rules.
# custom:
#   provider: azure
#   resources:
#     - azurerm_network_security_group
#     - azurerm_network_security_rule
package ergo.templates.azure.nsg_no_ssh_from_internet

import data.ergo.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("azurerm_network_security_group")
	tf.unknown(rc, ["security_rule"])
	msg := sprintf("%s security_rule is unknown at plan time", [rc.address])
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("azurerm_network_security_group")
	not tf.unknown(rc, ["security_rule"])
	some i, rule in object.get(rc.change.after, "security_rule", [])
	is_object(rule)
	tf.azure_exposed(rc, ["security_rule", i], 22)
	msg := tf.azure_expose_msg(rc, ["security_rule", i], "SSH")
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("azurerm_network_security_rule")
	tf.azure_exposed(rc, [], 22)
	msg := tf.azure_expose_msg(rc, [], "SSH")
}
