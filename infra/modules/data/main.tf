# 1. Amazon SQS FIFO (64 sharded MessageGroupIDs per event) + Dead Letter Queue
resource "aws_sqs_queue" "orders_dlq" {
  name                        = "${var.name_prefix}-orders-dlq.fifo"
  fifo_queue                  = true
  content_based_deduplication = false
  message_retention_seconds   = 1209600 # 14 days
  sqs_managed_sse_enabled     = true

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-orders-dlq.fifo"
  })
}

resource "aws_sqs_queue" "orders" {
  name                        = "${var.name_prefix}-orders.fifo"
  fifo_queue                  = true
  content_based_deduplication = false
  deduplication_scope         = "messageGroup"
  fifo_throughput_limit       = "perMessageGroupId"
  visibility_timeout_seconds  = 60
  receive_wait_time_seconds   = 20
  message_retention_seconds   = 345600 # 4 days
  sqs_managed_sse_enabled     = true

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.orders_dlq.arn
    maxReceiveCount     = 5
  })

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-orders.fifo"
  })
}

# 2. Amazon ElastiCache Redis 7 (Seat Lock Lua, ZSET Sweeper, Waiting Room & Pub/Sub)
resource "aws_elasticache_subnet_group" "redis" {
  name       = "${var.name_prefix}-redis-subnets"
  subnet_ids = var.private_subnet_ids

  tags = var.tags
}

resource "aws_security_group" "redis" {
  name        = "${var.name_prefix}-redis-sg"
  description = "Allow Redis 6379 inbound from private VPC/EKS CIDR blocks"
  vpc_id      = var.vpc_id

  ingress {
    description = "Redis TLS from VPC/EKS"
    from_port   = 6379
    to_port     = 6379
    protocol    = "tcp"
    cidr_blocks = var.allowed_cidr_blocks
  }

  egress {
    description = "Allow all outbound"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-redis-sg"
  })
}

resource "aws_elasticache_parameter_group" "redis" {
  name   = "${var.name_prefix}-redis7-params"
  family = "redis7"

  parameter {
    name  = "maxmemory-policy"
    value = "noeviction"
  }

  parameter {
    name  = "notify-keyspace-events"
    value = "Ex"
  }

  tags = var.tags
}

resource "aws_elasticache_replication_group" "redis" {
  replication_group_id       = "${var.name_prefix}-redis"
  description                = "KubeTicket Redis for Lua seat locks, waiting room, and WebSocket Pub/Sub"
  engine                     = "redis"
  engine_version             = "7.1"
  node_type                  = var.redis_node_type
  port                       = 6379
  num_cache_clusters         = var.redis_num_cache_clusters
  parameter_group_name       = aws_elasticache_parameter_group.redis.name
  subnet_group_name          = aws_elasticache_subnet_group.redis.name
  security_group_ids         = [aws_security_group.redis.id]
  automatic_failover_enabled = var.redis_num_cache_clusters > 1
  multi_az_enabled           = var.redis_num_cache_clusters > 1
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
  snapshot_retention_limit   = var.redis_num_cache_clusters > 1 ? 5 : 0

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-redis"
  })
}

# 3. AWS Secrets Manager metadata (secret values populated out-of-band; never stored in tfstate)
resource "aws_secretsmanager_secret" "app" {
  name                    = "${var.project_name}/${var.environment}/secrets"
  description             = "KubeTicket ${var.environment} runtime secrets (synced by External Secrets Operator)"
  recovery_window_in_days = var.secret_recovery_window_days

  tags = merge(var.tags, {
    Name = "${var.project_name}/${var.environment}/secrets"
  })
}
