# HashiCorp Vault with auto-unseal suing AWS KMS

This shows how to deploy Hashicorp Vault on Kubernetes and using auto-unseal feature with AWS KMS.

```bash
## create KMS and IAM Profile with policy to access KMS
pushd terraform
echo 'region = "us-west-2"' > terraform.tfvars
terraform init && terraform apply
popd

## set env vars for KMS and IAM Profile keys/secrets
. env.sh

## create secret
kubectl create secret generic vault -n vault \
    --from-literal=AWS_ACCESS_KEY_ID="${VAULT_AWS_ACCESS_KEY_ID?}" \
    --from-literal=AWS_SECRET_ACCESS_KEY="${VAULT_AWS_SECRET_ACCESS_KEY?}"

## set storage type ('raft' or 'consul')
export VAULT_STORAGE="raft" # or "consul"

## deploy vault
helmfile apply

## unseal
for POD in vault-{0..2}; do
  kubectl exec -it $POD -- vault operator init -n 1 -t 1
done
```
