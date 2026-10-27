# METADATA
# title: Azure SQL servers disable public network access
# description: Azure SQL servers set public_network_access_enabled to false.
# custom:
#   provider: azure
#   resources:
#     - azurerm_mssql_server
package ipso.templates.azure.sql_no_public_network

import data.ipso.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("azurerm_mssql_server")
	msg := tf.require_msg(rc, ["public_network_access_enabled"], false, "public_network_access_enabled")
}
