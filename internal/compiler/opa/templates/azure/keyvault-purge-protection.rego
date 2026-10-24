# METADATA
# title: Key vaults enable purge protection
# description: Key vaults set purge_protection_enabled.
# custom:
#   provider: azure
#   resources:
#     - azurerm_key_vault
package ergo.templates.azure.keyvault_purge_protection

import data.ergo.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("azurerm_key_vault")
	msg := tf.require_msg(rc, ["purge_protection_enabled"], true, "purge_protection_enabled")
}
