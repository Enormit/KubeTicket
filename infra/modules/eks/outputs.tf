output "cluster_name" {
  description = "EKS cluster name"
  value       = aws_eks_cluster.this.name
}

output "cluster_endpoint" {
  description = "EKS API server endpoint"
  value       = aws_eks_cluster.this.endpoint
}

output "oidc_provider_arn" {
  description = "EKS OIDC provider ARN for IRSA"
  value       = aws_iam_openid_connect_provider.eks.arn
}

output "app_irsa_role_arn" {
  description = "IAM Role ARN for Helm serviceAccount.annotations[\"eks.amazonaws.com/role-arn\"]"
  value       = aws_iam_role.app.arn
}

output "eso_irsa_role_arn" {
  description = "IAM Role ARN for External Secrets Operator"
  value       = aws_iam_role.eso.arn
}

output "alb_controller_role_arn" {
  description = "IAM Role ARN for AWS Load Balancer Controller"
  value       = aws_iam_role.alb_controller.arn
}
