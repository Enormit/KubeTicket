locals {
  environment  = "prod"
  name_prefix  = "${var.project_name}-${local.environment}"
  cluster_name = "${var.project_name}-${local.environment}-eks"
}

module "vpc" {
  source = "../../modules/vpc"

  name_prefix          = local.name_prefix
  cluster_name         = local.cluster_name
  vpc_cidr             = var.vpc_cidr
  azs                  = var.azs
  public_subnet_cidrs  = ["10.10.1.0/24", "10.10.2.0/24"]
  private_subnet_cidrs = ["10.10.11.0/24", "10.10.12.0/24"]
  single_nat_gateway   = false # Multi-AZ NAT HA for production
  enable_s3_endpoint   = true
}

module "ecr" {
  source = "../../modules/ecr"

  name_prefix          = "${var.project_name}-${local.environment}"
  image_tag_mutability = "IMMUTABLE"
  keep_image_count     = 20
  force_delete         = false
}

module "data" {
  source = "../../modules/data"

  project_name                = var.project_name
  environment                 = local.environment
  name_prefix                 = local.name_prefix
  vpc_id                      = module.vpc.vpc_id
  private_subnet_ids          = module.vpc.private_subnet_ids
  allowed_cidr_blocks         = [module.vpc.vpc_cidr_block]
  redis_node_type             = "cache.t4g.small"
  redis_num_cache_clusters    = 2 # Multi-AZ automatic failover
  secret_recovery_window_days = 7
}

module "eks" {
  source = "../../modules/eks"

  name_prefix              = local.name_prefix
  cluster_name             = local.cluster_name
  kubernetes_version       = "1.31"
  private_subnet_ids       = module.vpc.private_subnet_ids
  node_capacity_type       = "ON_DEMAND"
  node_instance_types      = var.node_instance_types
  node_desired_size        = var.node_desired_size
  node_min_size            = var.node_min_size
  node_max_size            = var.node_max_size
  app_namespace            = "kubeticket-prod"
  app_service_account_name = "kubeticket-prod"
  sqs_queue_arn            = module.data.sqs_queue_arn
  secrets_manager_arn      = module.data.secrets_manager_arn
}
