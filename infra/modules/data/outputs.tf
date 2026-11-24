output "sqs_queue_url" {
  description = "SQS FIFO URL for SQS_ORDER_QUEUE_URL and KEDA ScaledObject"
  value       = aws_sqs_queue.orders.url
}

output "sqs_queue_arn" {
  description = "SQS FIFO ARN for IAM IRSA least-privilege policies"
  value       = aws_sqs_queue.orders.arn
}

output "sqs_dlq_url" {
  description = "SQS FIFO Dead Letter Queue URL"
  value       = aws_sqs_queue.orders_dlq.url
}

output "redis_primary_endpoint" {
  description = "ElastiCache primary endpoint hostname"
  value       = aws_elasticache_replication_group.redis.primary_endpoint_address
}

output "redis_url" {
  description = "TLS Redis connection string for REDIS_URL"
  value       = "rediss://${aws_elasticache_replication_group.redis.primary_endpoint_address}:6379/0"
}

output "redis_security_group_id" {
  description = "ElastiCache Redis security group ID"
  value       = aws_security_group.redis.id
}

output "secrets_manager_arn" {
  description = "AWS Secrets Manager ARN synced by External Secrets Operator"
  value       = aws_secretsmanager_secret.app.arn
}

output "secrets_manager_name" {
  description = "AWS Secrets Manager name (matches Helm externalSecrets.remoteSecretName)"
  value       = aws_secretsmanager_secret.app.name
}
