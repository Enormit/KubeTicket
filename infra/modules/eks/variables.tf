variable "name_prefix" {
  description = "Resource prefix (e.g. kubeticket-dev or kubeticket-prod)"
  type        = string
}

variable "cluster_name" {
  description = "EKS cluster name"
  type        = string
}

variable "kubernetes_version" {
  description = "EKS Kubernetes version"
  type        = string
  default     = "1.31"
}

variable "private_subnet_ids" {
  description = "Private subnet IDs for EKS control plane ENIs and worker nodes"
  type        = list(string)
}

variable "endpoint_public_access" {
  description = "Enable public EKS API endpoint access"
  type        = bool
  default     = true
}

variable "node_capacity_type" {
  description = "Node group capacity type (SPOT for dev FinOps, ON_DEMAND for prod HA)"
  type        = string
  default     = "ON_DEMAND"

  validation {
    condition     = contains(["ON_DEMAND", "SPOT"], var.node_capacity_type)
    error_message = "node_capacity_type must be ON_DEMAND or SPOT."
  }
}

variable "node_instance_types" {
  description = "EC2 instance types for the EKS managed node group"
  type        = list(string)
  default     = ["t3.medium"]
}

variable "node_desired_size" {
  description = "Desired number of worker nodes"
  type        = number
  default     = 2
}

variable "node_min_size" {
  description = "Minimum number of worker nodes"
  type        = number
  default     = 2
}

variable "node_max_size" {
  description = "Maximum number of worker nodes"
  type        = number
  default     = 4
}

variable "app_namespace" {
  description = "Kubernetes namespace where KubeTicket Helm release runs"
  type        = string
}

variable "app_service_account_name" {
  description = "Kubernetes ServiceAccount name bound to App IRSA role"
  type        = string
}

variable "sqs_queue_arn" {
  description = "SQS FIFO queue ARN accessed by flashsale-engine, order-worker, and KEDA"
  type        = string
}

variable "secrets_manager_arn" {
  description = "AWS Secrets Manager ARN read by External Secrets Operator"
  type        = string
}

variable "tags" {
  description = "Additional tags applied to EKS and IAM resources"
  type        = map(string)
  default     = {}
}
