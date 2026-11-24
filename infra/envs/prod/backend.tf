# Run `infra/bootstrap/` first, then initialize with:
#   terraform init -backend-config="bucket=<STATE_BUCKET>" -backend-config="dynamodb_table=kubeticket-tfstate-locks" -backend-config="region=ap-southeast-1"
terraform {
  backend "s3" {
    key     = "kubeticket/prod/terraform.tfstate"
    encrypt = true
  }
}
