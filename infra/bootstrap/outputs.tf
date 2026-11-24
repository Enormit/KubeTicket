output "state_bucket_name" {
  description = "S3 bucket name for Terraform remote state"
  value       = aws_s3_bucket.tfstate.id
}

output "lock_table_name" {
  description = "DynamoDB table name for Terraform state locks"
  value       = aws_dynamodb_table.locks.name
}

output "gitlab_ci_role_arn" {
  description = "IAM Role ARN for GitLab CI OIDC ECR push (null if enable_gitlab_oidc=false)"
  value       = var.enable_gitlab_oidc ? aws_iam_role.gitlab_ci[0].arn : null
}

output "backend_config_example" {
  description = "Paste into infra/envs/<env>/backend.tf"
  value       = <<-EOT
    bucket         = "${aws_s3_bucket.tfstate.id}"
    key            = "kubeticket/<env>/terraform.tfstate"
    region         = "${var.aws_region}"
    dynamodb_table = "${aws_dynamodb_table.locks.name}"
    encrypt        = true
  EOT
}
