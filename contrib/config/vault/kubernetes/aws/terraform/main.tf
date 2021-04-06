resource "aws_kms_key" "vault" {
  description             = "Vault unseal key"
  deletion_window_in_days = 10

  tags = {
    Name = "vault-kms-unseal-${random_pet.env.id}"
  }
}

resource "random_pet" "env" {
  length    = 2
  separator = "_"
}

resource "local_file" "env_sh" {
  count           = var.create_env_sh != "" ? 1 : 0
  content         = local.env_sh
  filename        = "${path.module}/../env.sh"
  file_permission = "0644"
}

# resource "aws_iam_role" "vault-kms-unseal" {
#   count         = var.user_enabled ? 1 : 0
#   name               = "vault-kms-role-${random_pet.env.id}"
#   assume_role_policy = data.aws_iam_policy_document.assume_role.json
# }

## IAM User
resource "aws_iam_user" "vault-kms-unseal" {
  count         = var.user_enabled ? 1 : 0
  name          = "vault-kms-role-${random_pet.env.id}"
  path          = var.path
  force_destroy = var.force_destroy
}

## Generate AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY
resource "aws_iam_access_key" "vault-kms-unseal" {
  count = var.user_enabled && var.access_key_enabled ? 1 : 0
  user  = aws_iam_user.vault-kms-unseal[0].name
}

## Add Policy to User
resource "aws_iam_user_policy" "vault-kms-unseal" {
  count  = var.user_enabled ? 1 : 0
  name   = "vault-kms-role-${random_pet.env.id}"
  user   = aws_iam_user.vault-kms-unseal[0].name
  policy = data.aws_iam_policy_document.vault-kms-unseal.json
}
