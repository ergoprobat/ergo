# METADATA
# title: Storage accounts accept HTTPS only
# description: Storage accounts set https_traffic_only_enabled. azurerm 5.x defaults this to true only when the plan shows it.
# custom:
#   provider: azure
#   resources:
#     - azurerm_storage_account
package ipso.templates.azure.storage_https_only

import data.ipso.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("azurerm_storage_account")
	msg := tf.require_msg(rc, ["https_traffic_only_enabled"], true, "https_traffic_only_enabled")
}
