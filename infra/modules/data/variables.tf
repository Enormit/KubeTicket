variable "project_name" {
  description = "Project prefix (used for Secrets Manager path)"
  type        = string
  default     = "kubeticket"
}

variable "environment" {
  description = "Environment name (dev or prod)"
  type        = string
}

variable "name_prefix" {
  description = "Resource prefix (e.g. kubeticket-dev or kubeticket-prod)"
  type        = string
}

variable "vpc_id" {
  description = "VPC ID where ElastiCache security group is created"
  type        = string
}

variable "private_subnet_ids" {
  description = "Private subnet IDs for ElastiCache Redis subnet group"
  type        = list(string)
}

variable "allowed_cidr_blocks" {
  description = "VPC/Pod CIDR blocks allowed to connect to ElastiCache Redis on port 6379"
  type        = list(string)
}

variable "redis_node_type" {
  description = "ElastiCache node type (cache.t4g.micro for dev, cache.t4g.small+ for prod)"
  type        = string
  default     = "cache.t4g.micro"
}

variable "redis_num_cache_clusters" {
  description = "Number of Redis cache nodes (1 for dev, 2+ for Multi-AZ failover in prod)"
  type        = number
  default     = 1
}

variable "secret_recovery_window_days" {
  description = "Secrets Manager recovery window in days (0 allows immediate delete in dev)"
  type        = number
  default     = 7
}

variable "tags" {
  description = "Additional tags applied to data resources"
  type        = map(string)
  default     = {}
}
