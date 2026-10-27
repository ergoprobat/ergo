# METADATA
# title: Storage accounts disallow public blob access
# description: Storage accounts set allow_nested_items_to_be_public to false.
# custom:
#   provider: azure
#   resources:
#     - azurerm_storage_account
package ipso.templates.azure.storage_no_public_blob

import data.ipso.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("azurerm_storage_account")
	msg := tf.require_msg(rc, ["allow_nested_items_to_be_public"], false, "allow_nested_items_to_be_public")
}
