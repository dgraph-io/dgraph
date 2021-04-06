locals {
  env_vars = {
    kms_key        = aws_kms_key.vault.id
    aws_region     = var.region
    aws_access_key = aws_iam_access_key.vault-kms-unseal[0].id
    aws_secret_key = aws_iam_access_key.vault-kms-unseal[0].secret
  }

  env_sh = templatefile("${path.module}/templates/env.sh.tmpl", local.env_vars)
}
