data "aws_caller_identity" "current" {}

locals {
  resolved_bucket_name = var.state_bucket_name != "" ? var.state_bucket_name : "${var.project_name}-tfstate-${data.aws_caller_identity.current.account_id}-${var.aws_region}"
}

resource "aws_s3_bucket" "tfstate" {
  bucket = local.resolved_bucket_name

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_versioning" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_dynamodb_table" "locks" {
  name         = var.lock_table_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }

  point_in_time_recovery {
    enabled = true
  }
}

data "tls_certificate" "gitlab" {
  count = var.enable_gitlab_oidc ? 1 : 0
  url   = "https://gitlab.com"
}

resource "aws_iam_openid_connect_provider" "gitlab" {
  count = var.enable_gitlab_oidc ? 1 : 0

  url             = "https://gitlab.com"
  client_id_list  = ["https://gitlab.com"]
  thumbprint_list = [data.tls_certificate.gitlab[0].certificates[0].sha1_fingerprint]
}

data "aws_iam_policy_document" "gitlab_assume_role" {
  count = var.enable_gitlab_oidc ? 1 : 0

  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.gitlab[0].arn]
    }

    condition {
      test     = "StringEquals"
      variable = "gitlab.com:aud"
      values   = ["https://gitlab.com"]
    }

    condition {
      test     = "StringLike"
      variable = "gitlab.com:sub"
      values = distinct([
        "project_path:${var.gitlab_project_path}:ref_type:*",
        "project_path:${lower(var.gitlab_project_path)}:ref_type:*",
      ])
    }
  }
}

resource "aws_iam_role" "gitlab_ci" {
  count = var.enable_gitlab_oidc ? 1 : 0

  name               = "${var.project_name}-gitlab-ci-ecr"
  assume_role_policy = data.aws_iam_policy_document.gitlab_assume_role[0].json
}

resource "aws_iam_role_policy_attachment" "gitlab_ecr" {
  count = var.enable_gitlab_oidc ? 1 : 0

  role       = aws_iam_role.gitlab_ci[0].name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPowerUser"
}

data "tls_certificate" "github" {
  count = var.enable_github_oidc ? 1 : 0
  url   = "https://token.actions.githubusercontent.com"
}

resource "aws_iam_openid_connect_provider" "github" {
  count = var.enable_github_oidc ? 1 : 0

  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.github[0].certificates[0].sha1_fingerprint]
}

data "aws_iam_policy_document" "github_assume_role" {
  count = var.enable_github_oidc ? 1 : 0

  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github[0].arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values = distinct([
        "repo:${var.github_repo}:*",
        "repo:${lower(var.github_repo)}:*",
      ])
    }
  }
}

resource "aws_iam_role" "github_ci" {
  count = var.enable_github_oidc ? 1 : 0

  name               = "${var.project_name}-github-ci-ecr"
  assume_role_policy = data.aws_iam_policy_document.github_assume_role[0].json
}

resource "aws_iam_role_policy_attachment" "github_ecr" {
  count = var.enable_github_oidc ? 1 : 0

  role       = aws_iam_role.github_ci[0].name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPowerUser"
}
