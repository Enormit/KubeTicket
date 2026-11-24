variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "ap-southeast-1"
}

variable "project_name" {
  description = "Project prefix"
  type        = string
  default     = "kubeticket"
}

variable "vpc_cidr" {
  description = "VPC CIDR block"
  type        = string
  default     = "10.0.0.0/16"
}

variable "azs" {
  description = "Availability Zones"
  type        = list(string)
  default     = ["ap-southeast-1a", "ap-southeast-1b"]
}

variable "node_instance_types" {
  description = "EKS worker EC2 instance types"
  type        = list(string)
  default     = ["t3.medium"]
}

variable "node_desired_size" {
  description = "Desired EKS worker node count"
  type        = number
  default     = 2
}

variable "node_min_size" {
  description = "Minimum EKS worker node count"
  type        = number
  default     = 2
}

variable "node_max_size" {
  description = "Maximum EKS worker node count"
  type        = number
  default     = 4
}
