# METADATA
# title: Storage accounts require TLS 1.2
# description: Storage accounts set min_tls_version to TLS1_2. azurerm 5.x accepts TLS1_2 as the minimum.
# custom:
#   provider: azure
#   resources:
#     - azurerm_storage_account
package ipso.templates.azure.storage_min_tls_1_2

import data.ipso.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("azurerm_storage_account")
	msg := tf.require_msg(rc, ["min_tls_version"], "TLS1_2", "min_tls_version")
}
