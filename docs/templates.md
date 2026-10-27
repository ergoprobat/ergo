# Terraform plan templates

A check with `engine: opa` and `ref: ipso:<id>` compiles one of these templates. The check `target` must be `terraform-plan`. Each template reads a Terraform or OpenTofu plan and denies with an object that carries `msg` and `address`.

Schemas are hashicorp/aws 6.x and hashicorp/azurerm 5.x.

| id | title | resources |
| --- | --- | --- |
| `aws/ebs-encrypted` | EBS volumes are encrypted | aws_ebs_volume, aws_instance, aws_launch_template |
| `aws/ec2-imdsv2-required` | EC2 instances require IMDSv2 | aws_instance, aws_launch_template |
| `aws/kms-key-rotation` | Symmetric KMS keys rotate | aws_kms_key |
| `aws/lb-http-redirects-to-https` | HTTP listeners redirect to HTTPS | aws_lb_listener |
| `aws/rds-not-public` | RDS instances are not publicly accessible | aws_db_instance |
| `aws/rds-storage-encrypted` | RDS storage is encrypted | aws_db_instance, aws_rds_cluster |
| `aws/s3-kms-cmk` | S3 buckets use a customer-managed KMS key | aws_s3_bucket, aws_s3_bucket_server_side_encryption_configuration |
| `aws/s3-no-public-acl` | S3 bucket ACLs are not public | aws_s3_bucket, aws_s3_bucket_acl |
| `aws/s3-public-access-block` | S3 buckets block public access | aws_s3_bucket, aws_s3_bucket_public_access_block |
| `aws/sg-no-rdp-from-internet` | Security groups do not allow RDP from the internet | aws_security_group, aws_security_group_rule, aws_vpc_security_group_ingress_rule |
| `aws/sg-no-ssh-from-internet` | Security groups do not allow SSH from the internet | aws_security_group, aws_security_group_rule, aws_vpc_security_group_ingress_rule |
| `azure/aks-private-cluster` | AKS clusters use a private API server | azurerm_kubernetes_cluster |
| `azure/keyvault-purge-protection` | Key vaults enable purge protection | azurerm_key_vault |
| `azure/nsg-no-rdp-from-internet` | Network security groups do not allow RDP from the internet | azurerm_network_security_group, azurerm_network_security_rule |
| `azure/nsg-no-ssh-from-internet` | Network security groups do not allow SSH from the internet | azurerm_network_security_group, azurerm_network_security_rule |
| `azure/sql-no-public-network` | Azure SQL servers disable public network access | azurerm_mssql_server |
| `azure/storage-https-only` | Storage accounts accept HTTPS only | azurerm_storage_account |
| `azure/storage-min-tls-1-2` | Storage accounts require TLS 1.2 | azurerm_storage_account |
| `azure/storage-no-public-blob` | Storage accounts disallow public blob access | azurerm_storage_account |
| `terraform/postgres-only` | New datastores are PostgreSQL | aws_db_instance, aws_rds_cluster, aws_dynamodb_table, aws_docdb_cluster, aws_neptune_cluster, aws_redshift_cluster, aws_elasticache_cluster, aws_elasticache_replication_group, aws_memorydb_cluster, azurerm_mssql_server, azurerm_mysql_flexible_server, azurerm_cosmosdb_account, azurerm_redis_cache, azurerm_postgresql_flexible_server |
