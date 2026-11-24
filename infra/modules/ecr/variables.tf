variable "name_prefix" {
  description = "Repository namespace prefix (e.g. kubeticket)"
  type        = string
  default     = "kubeticket"
}

variable "repository_names" {
  description = "Container image repositories created for KubeTicket microservices and migrations"
  type        = list(string)
  default     = ["gateway-api", "flashsale-engine", "order-worker", "frontend", "db-migrator"]
}

variable "image_tag_mutability" {
  description = "ECR tag mutability (MUTABLE for dev, IMMUTABLE for prod)"
  type        = string
  default     = "IMMUTABLE"

  validation {
    condition     = contains(["MUTABLE", "IMMUTABLE"], var.image_tag_mutability)
    error_message = "image_tag_mutability must be MUTABLE or IMMUTABLE."
  }
}

variable "keep_image_count" {
  description = "Number of tagged images to retain per repository"
  type        = number
  default     = 10
}

variable "force_delete" {
  description = "Allow deleting non-empty ECR repositories on destroy (true for dev, false for prod)"
  type        = bool
  default     = false
}

variable "tags" {
  description = "Additional tags applied to ECR repositories"
  type        = map(string)
  default     = {}
}
