variable "aws_region" {
  description = "AWS region for remote Terraform state storage"
  type        = string
  default     = "ap-southeast-1"
}

variable "project_name" {
  description = "Project prefix used for resource naming and tags"
  type        = string
  default     = "kubeticket"
}

variable "state_bucket_name" {
  description = "Optional custom S3 bucket name for Terraform state (auto-generated with account ID if empty)"
  type        = string
  default     = ""
}

variable "lock_table_name" {
  description = "DynamoDB table name for Terraform state locking"
  type        = string
  default     = "kubeticket-tfstate-locks"
}

variable "enable_gitlab_oidc" {
  description = "Create an IAM OIDC provider and role for GitLab CI/CD keyless AWS authentication"
  type        = bool
  default     = false
}

variable "gitlab_project_path" {
  description = "GitLab group/project path allowed to assume the OIDC CI role"
  type        = string
  default     = "Enormit/kubeticket"
}

variable "enable_github_oidc" {
  description = "Create an IAM OIDC provider and role for GitHub Actions keyless AWS authentication"
  type        = bool
  default     = false
}

variable "github_repo" {
  description = "GitHub owner/repo allowed to assume the OIDC CI role"
  type        = string
  default     = "Enormit/KubeTicket"
}
