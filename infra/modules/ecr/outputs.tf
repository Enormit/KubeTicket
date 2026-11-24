output "repository_urls" {
  description = "Map of service name to ECR repository URL"
  value       = { for k, repo in aws_ecr_repository.this : k => repo.repository_url }
}

output "repository_arns" {
  description = "Map of service name to ECR repository ARN"
  value       = { for k, repo in aws_ecr_repository.this : k => repo.arn }
}
