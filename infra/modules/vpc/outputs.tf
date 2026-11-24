output "vpc_id" {
  description = "VPC ID"
  value       = aws_vpc.this.id
}

output "vpc_cidr_block" {
  description = "VPC CIDR block"
  value       = aws_vpc.this.cidr_block
}

output "public_subnet_ids" {
  description = "Public subnet IDs (for ALB and NAT Gateways)"
  value       = aws_subnet.public[*].id
}

output "private_subnet_ids" {
  description = "Private subnet IDs (for EKS nodes and ElastiCache)"
  value       = aws_subnet.private[*].id
}

output "private_route_table_ids" {
  description = "Private route table IDs"
  value       = aws_route_table.private[*].id
}

output "nat_public_ips" {
  description = "Elastic IPs attached to NAT Gateways"
  value       = aws_eip.nat[*].public_ip
}

output "s3_endpoint_id" {
  description = "S3 VPC Gateway Endpoint ID (null if disabled)"
  value       = var.enable_s3_endpoint ? aws_vpc_endpoint.s3[0].id : null
}
