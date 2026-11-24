locals {
  environment  = "dev"
  name_prefix  = "${var.project_name}-${local.environment}"
  cluster_name = "${var.project_name}-${local.environment}-eks"
}

module "vpc" {
  source = "../../modules/vpc"

  name_prefix          = local.name_prefix
  cluster_name         = local.cluster_name
  vpc_cidr             = var.vpc_cidr
  azs                  = var.azs
  public_subnet_cidrs  = ["10.0.1.0/24", "10.0.2.0/24"]
  private_subnet_cidrs = ["10.0.11.0/24", "10.0.12.0/24"]
  single_nat_gateway   = true # FinOps: single NAT Gateway for dev
  enable_s3_endpoint   = true
}

module "ecr" {
  source = "../../modules/ecr"

  name_prefix          = "${var.project_name}-${local.environment}"
  image_tag_mutability = "MUTABLE"
  keep_image_count     = 10
  force_delete         = true
}

module "data" {
  source = "../../modules/data"

  project_name                = var.project_name
  environment                 = local.environment
  name_prefix                 = local.name_prefix
  vpc_id                      = module.vpc.vpc_id
  private_subnet_ids          = module.vpc.private_subnet_ids
  allowed_cidr_blocks         = [module.vpc.vpc_cidr_block]
  redis_node_type             = "cache.t4g.micro"
  redis_num_cache_clusters    = 1
  secret_recovery_window_days = 0
}

module "eks" {
  source = "../../modules/eks"

  name_prefix              = local.name_prefix
  cluster_name             = local.cluster_name
  kubernetes_version       = "1.31"
  private_subnet_ids       = module.vpc.private_subnet_ids
  node_capacity_type       = "SPOT" # FinOps: Spot worker nodes for dev
  node_instance_types      = var.node_instance_types
  node_desired_size        = var.node_desired_size
  node_min_size            = var.node_min_size
  node_max_size            = var.node_max_size
  app_namespace            = "kubeticket-dev"
  app_service_account_name = "kubeticket-dev"
  sqs_queue_arn            = module.data.sqs_queue_arn
  secrets_manager_arn      = module.data.secrets_manager_arn
}
