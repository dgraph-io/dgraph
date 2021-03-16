# HashiCorp Vault Integration

This is a small guide and sample scripts to get started on your Vault journey with Dgraph.

Overview:
1. Launch unsealed Vault server
2. Enable approle and secrets kv
3. Create the admin role with an attached policy
4. Retrieve the admin token
5. Create the dgraph role with an attached policy
6. Save secrets using admin role
7. Retrieve the dgraph token and save role_id and secret_id
7. Verify access using dgraph secrets
8. Launch dgraph

## Prerequisites

* [Docker](https://docs.docker.com/engine/install/)
* [Docker Compose](https://docs.docker.com/compose/install/)
* [jq](https://stedolan.github.io/jq/)
* [curl](https://curl.se/)

## Steps

### Launch unsealed Vault server

```bash
## launch vault server
docker-compose up --detach "vault"

## initialize vault and copy secrets down
docker exec -t vault vault operator init

## unseal vault using copied secrets
docker exec -ti vault vault operator unseal
docker exec -ti vault vault operator unseal
docker exec -ti vault vault operator unseal
```

### Enable approle and secrets kv

Using the root token copied from `vault operator init`, we can enabled these features:

```bash
export VAULT_ADDRESS="127.0.0.1:8200"

curl --silent --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  --request POST \
  --data '{"type": "approle"}' \
  $VAULT_ADDRESS/v1/sys/auth/approle

curl --silent --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  --request POST \
  --data '{ "type": "kv-v2" }' \
  $VAULT_ADDRESS/v1/sys/mounts/secret  
```

### Create the admin role with an attached policy

```bash
## convert policies to json format
cat <<EOF > ./vault/policy_admin.json
{
  "policy": "$(sed -e ':a;N;$!ba;s/\n/\\n/g' -e 's/"/\\"/g' vault/policy_admin.hcl)"
}
EOF

## create the admin policy
curl --silent --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  --request PUT --data @./vault/policy_admin.json \
  http://$VAULT_ADDRESS/v1/sys/policies/acl/admin

## create the admin role with an attached policy
curl --silent --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
 --request POST \
 --data '{ "token_policies": "admin", "token_ttl": "1h", "token_max_ttl": "4h" }' \
 http://$VAULT_ADDRESS/v1/auth/approle/role/admin

## verify the role
curl --silent --header "X-Vault-Token: $VAULT_ROOT_TOKEN" --request GET \
 http://$VAULT_ADDRESS/v1/auth/approle/role/admin | jq
```

### Retrieve the admin token

From here, we'll want to get a admin token that we can use for the rest of the process:

```bash
VAULT_ADMIN_ROLE_ID=$(curl --silent --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph/role-id | jq -r '.data.role_id'
)

curl --silent --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  --request POST \
  http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph/secret-id | jq

export VAULT_ADMIN_TOKEN=$(curl --silent --request POST \
  --data "{ \"role_id\": \"$VAULT_ADMIN_ROLE_ID\", \"secret_id\": \"$VAULT_ADMIN_SECRET_ID\" }" \
  http://$VAULT_ADDRESS/v1/auth/approle/login | jq -r '.auth.client_token'
)
```

### Create the dgraph role with an attached policy

```bash
## convert policies to json format
cat <<EOF > ./vault/policy_dgraph.json
{
  "policy": "$(sed -e ':a;N;$!ba;s/\n/\\n/g' -e 's/"/\\"/g' vault/policy_dgraph.hcl)"
}
EOF

## create the dgraph policy
curl --silent --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
  --request PUT --data @./vault/policy_dgraph.json \
  http://$VAULT_ADDRESS/v1/sys/policies/acl/dgraph


## create the dgraph role with an attached policy
curl --silent --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
 --request POST \
 --data '{ "token_policies": "dgraph", "token_ttl": "1h", "token_max_ttl": "4h" }' \
 http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph

## verify the role
curl --silent --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" --request GET \
 http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph | jq
```

### Save secrets using admin role

```bash
curl --silent \
  --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
  --request POST \
  --data @./vault/payload_enc_key.json \
  http://$VAULT_ADDRESS/v1/secret/data/dgraph/enc_key
```

### Retrieve the dgraph token and save role_id and secret_id

```bash
VAULT_DGRAPH_ROLE_ID=$(curl --silent --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
  http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph/role-id | jq -r '.data.role_id'
)

curl --silent --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
  --request POST \
  http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph/secret-id | jq

export VAULT_DGRAPH_TOKEN=$(curl --silent --request POST \
  --data "{ \"role_id\": \"$VAULT_DGRAPH_ROLE_ID\", \"secret_id\": \"$VAULT_DGRAPH_SECRET_ID\" }" \
  http://$VAULT_ADDRESS/v1/auth/approle/login | jq -r '.auth.client_token'
)
```

Also, we want to save the role-id and secret-id for the Dgraph Alpha server.

```bash
echo $VAULT_DGRAPH_ROLE_ID > ./vault/role_id
echo $VAULT_DGRAPH_SECRET_ID > ./vault/secret_id
```

### Verify access using dgraph secrets

```bash
curl --silent \
  --header "X-Vault-Token: $VAULT_DGRAPH_TOKEN" \
  --request GET \
  http://$VAULT_ADDRESS/v1/secret/data/dgraph/enc_key | jq
```

### Launch Dgraph

```bash
docker-compose up --detach
```
