data "aws_caller_identity" "current" {}

locals {
  environment       = "dev"
  name_prefix       = "${var.project_name}-${local.environment}"
  cluster_name      = "${var.project_name}-${local.environment}-eks"
  enable_custom_dns = var.route53_zone_name != "" && var.domain_name != ""
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

data "aws_route53_zone" "this" {
  count = local.enable_custom_dns ? 1 : 0

  name         = var.route53_zone_name
  private_zone = false
}

resource "aws_acm_certificate" "app" {
  count = local.enable_custom_dns ? 1 : 0

  domain_name       = var.domain_name
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "acm_validation" {
  for_each = local.enable_custom_dns ? {
    for dvo in aws_acm_certificate.app[0].domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  } : {}

  allow_overwrite = true
  name            = each.value.name
  records         = [each.value.record]
  ttl             = 60
  type            = each.value.type
  zone_id         = data.aws_route53_zone.this[0].zone_id
}

resource "aws_acm_certificate_validation" "app" {
  count = local.enable_custom_dns && var.validate_acm_certificate ? 1 : 0

  certificate_arn         = aws_acm_certificate.app[0].arn
  validation_record_fqdns = [for record in aws_route53_record.acm_validation : record.fqdn]
}
