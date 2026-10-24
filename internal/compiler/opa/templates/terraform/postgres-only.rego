# METADATA
# title: New datastores are PostgreSQL
# description: AWS and Azure database resources are PostgreSQL. DynamoDB, DocumentDB, Neptune, Redshift, ElastiCache, MemoryDB, Azure SQL, MySQL, Cosmos DB, and Azure Cache for Redis are denied.
# custom:
#   provider: terraform
#   resources:
#     - aws_db_instance
#     - aws_rds_cluster
#     - aws_dynamodb_table
#     - aws_docdb_cluster
#     - aws_neptune_cluster
#     - aws_redshift_cluster
#     - aws_elasticache_cluster
#     - aws_elasticache_replication_group
#     - aws_memorydb_cluster
#     - azurerm_mssql_server
#     - azurerm_mysql_flexible_server
#     - azurerm_cosmosdb_account
#     - azurerm_redis_cache
#     - azurerm_postgresql_flexible_server
package ergo.templates.terraform.postgres_only

import data.ergo.lib.terraform as tf

other_engines := {
	"aws_dynamodb_table": "DynamoDB",
	"aws_docdb_cluster": "DocumentDB",
	"aws_neptune_cluster": "Neptune",
	"aws_redshift_cluster": "Redshift",
	"aws_elasticache_cluster": "ElastiCache",
	"aws_elasticache_replication_group": "ElastiCache",
	"aws_memorydb_cluster": "MemoryDB",
	"azurerm_mssql_server": "Azure SQL",
	"azurerm_mysql_flexible_server": "Azure MySQL",
	"azurerm_cosmosdb_account": "Cosmos DB",
	"azurerm_redis_cache": "Azure Cache for Redis",
}

deny contains tf.violation(rc, msg) if {
	some type, label in other_engines
	some rc in tf.resources(type)
	msg := sprintf("%s is %s, not PostgreSQL", [rc.address, label])
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_db_instance")
	msg := tf.require_msg(rc, ["engine"], "postgres", "engine")
}

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_rds_cluster")
	msg := tf.require_msg(rc, ["engine"], "aurora-postgresql", "engine")
}
