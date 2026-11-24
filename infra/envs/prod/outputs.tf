output "eks_cluster_name" {
  value = module.eks.cluster_name
}

output "kubeconfig_command" {
  value = "aws eks update-kubeconfig --region ${var.aws_region} --name ${module.eks.cluster_name}"
}

output "ecr_repository_urls" {
  value = module.ecr.repository_urls
}

output "sqs_queue_url" {
  value = module.data.sqs_queue_url
}

output "redis_url" {
  value = module.data.redis_url
}

output "secrets_manager_name" {
  value = module.data.secrets_manager_name
}

output "app_irsa_role_arn" {
  value = module.eks.app_irsa_role_arn
}

output "eso_irsa_role_arn" {
  value = module.eks.eso_irsa_role_arn
}

output "alb_controller_role_arn" {
  value = module.eks.alb_controller_role_arn
}
