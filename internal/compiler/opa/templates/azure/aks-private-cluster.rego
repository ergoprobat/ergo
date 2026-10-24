# METADATA
# title: AKS clusters use a private API server
# description: AKS clusters set private_cluster_enabled. The provider default is a public API server.
# custom:
#   provider: azure
#   resources:
#     - azurerm_kubernetes_cluster
package ergo.templates.azure.aks_private_cluster

import data.ergo.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("azurerm_kubernetes_cluster")
	msg := tf.require_msg(rc, ["private_cluster_enabled"], true, "private_cluster_enabled")
}
