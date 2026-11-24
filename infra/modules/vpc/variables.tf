variable "name_prefix" {
  description = "Resource name prefix (e.g. kubeticket-dev or kubeticket-prod)"
  type        = string
}

variable "cluster_name" {
  description = "EKS cluster name used for AWS Load Balancer Controller subnet discovery tags"
  type        = string
}

variable "vpc_cidr" {
  description = "Primary CIDR block for the VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "azs" {
  description = "Availability Zones for Multi-AZ deployment (at least 2 required for EKS/ALB)"
  type        = list(string)
  default     = ["ap-southeast-1a", "ap-southeast-1b"]

  validation {
    condition     = length(var.azs) >= 2
    error_message = "At least 2 Availability Zones are required for EKS and ALB."
  }
}

variable "public_subnet_cidrs" {
  description = "CIDR blocks for public subnets (ALB + NAT Gateways), 1 per AZ"
  type        = list(string)
  default     = ["10.0.1.0/24", "10.0.2.0/24"]

  validation {
    condition     = length(var.public_subnet_cidrs) == length(var.azs)
    error_message = "public_subnet_cidrs length must match azs length."
  }
}

variable "private_subnet_cidrs" {
  description = "CIDR blocks for private subnets (EKS Nodes + ElastiCache), 1 per AZ"
  type        = list(string)
  default     = ["10.0.11.0/24", "10.0.12.0/24"]

  validation {
    condition     = length(var.private_subnet_cidrs) == length(var.azs)
    error_message = "private_subnet_cidrs length must match azs length."
  }
}

variable "single_nat_gateway" {
  description = "Use 1 shared NAT Gateway (FinOps for dev) instead of 1 NAT Gateway per AZ (HA for prod)"
  type        = bool
  default     = true
}

variable "enable_s3_endpoint" {
  description = "Create a free S3 VPC Gateway Endpoint so ECR layer pulls and log exports bypass NAT cost"
  type        = bool
  default     = true
}

variable "tags" {
  description = "Additional tags applied to all VPC resources"
  type        = map(string)
  default     = {}
}
