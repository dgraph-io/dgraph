# HashiCorp Vault Integration: Docker

This shows how to set up a local staging server for HashiCorp Vault and Dgraph. Through these steps below, you can create secrets for [Encryption at Rest](https://dgraph.io/docs/enterprise-features/encryption-at-rest/) and [Access Control Lists](https://dgraph.io/docs/enterprise-features/access-control-lists/).  You can change the example secrets in [vault/payload_alpha_secrets.json](vault/payload_alpha_secrets.json) file.

This guide will demonstrate using best practices with two personas:

* `admin` persona with privileged permissions to configure an auth method
* `app` persona (`dgraph`) - a consumer of secrets stored in Vault

Steps using `bind_secret_id`:

1.  [Configure Dgraph and Vault Versions](#Step-1-configure-dgraph-and-vault-versions)
2.  [Launch unsealed Vault server](#Step-2-launch-unsealed-Vault-server)
3.  [Enable AppRole Auth and KV Secrets](#Step-3-enable-AppRole-Auth-and-KV-Secrets)
4.  [Create an `admin` policy](#Step-4-create-an-admin-policy)
5.  [Create an `admin` role with the attached policy](#Step-5-create-an-admin-role-with-the-attached-policy)
6.  [Retrieve the admin token](#Step-6-retrieve-the-admin-token)
7.  [Create a `dgraph` policy to access the secrets](#Step-7-create-a-dgraph-policy-to-access-the-secrets)
8.  [Create a `dgraph` role with the attached policy](#Step-8-create-a-dgraph-role-with-the-attached-policy)
9.  [Save secrets using admin persona](#Step-9-save-secrets-using-admin-persona)
10. [Retrieve the `dgraph` token and save credentials](#Step-10-retrieve-the-dgraph-token-and-save-credentials)
11. [Verify secrets access using app persona](#Step-11-verify-secrets-access-using-app-persona)
12. [Launch Dgraph](#Step-12-launch-Dgraph)

Alternative Steps using `bound_cidr_list` (see [Using Hashicorp Vault CIDR List for Authentication](#Using-hashicorp-vault-cidr-list-for-authentication)):

1.  [Configure Dgraph and Vault Versions](#Step-1-configure-dgraph-and-vault-versions)
2.  [Launch unsealed Vault server](#Step-2-launch-unsealed-Vault-server)
3.  [Enable AppRole Auth and KV Secrets](#Step-3-enable-AppRole-Auth-and-KV-Secrets)
4.  [Create an `admin` policy](#Step-4-create-an-admin-policy)
5.  [Create an `admin` role with the attached policy](#Step-5-create-an-admin-role-with-the-attached-policy)
6.  [Retrieve the admin token](#Step-6-retrieve-the-admin-token)
7.  [Create a `dgraph` policy to access the secrets](#Step-7-create-a-dgraph-policy-to-access-the-secrets)
8.  [Create a `dgraph` role using `bound_cidr_list`](#Step-8-create-a-dgraph-role-using-bound_cidr_list)
9.  [Save secrets using admin persona](#Step-9-save-secrets-using-admin-persona)
10. [Retrieve the dgraph token using only the `role-id`](#Step-10-retrieve-the-dgraph-token-using-only-the-role-id)
11. [Verify secrets access using app persona](#Step-11-verify-secrets-access-using-app-persona)
12. [Launch Dgraph](#Step-12-launch-Dgraph)

## Prerequisites

* [Docker](https://docs.docker.com/engine/install/)
* [Docker Compose](https://docs.docker.com/compose/install/)
* [jq](https://stedolan.github.io/jq/)
* [curl](https://curl.se/)

## Steps

This configures an app role that requires log in with `role-id` and `secret-id` to login.  This is the default role setting where `bind_seccret_id` is enabled.

### Step 1: Configure Dgraph and Vault Versions

```bash
export DGRAPH_VERSION="v21.03"  # default is 'latest'
export VAULT_VERSION="1.7.0"    # default is 'latest'
```

**NOTE**: This guide has been tested with Hashicorp Vault version `1.6.3` and `1.7.0`.

### Step 2: Launch unsealed Vault server

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

### Step 3: Enable AppRole Auth and KV Secrets

Using the root token copied from `vault operator init`, we can enable these features:

```bash
export VAULT_ROOT_TOKEN="<root-token>"
```

```bash
export VAULT_ADDRESS="127.0.0.1:8200"

curl --silent \
  --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  --request POST \
  --data '{"type": "approle"}' \
  $VAULT_ADDRESS/v1/sys/auth/approle

curl --silent \
  --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  --request POST \
  --data '{ "type": "kv-v2" }' \
  $VAULT_ADDRESS/v1/sys/mounts/secret
```

### Step 4: Create an `admin` policy

```bash
## convert policies to json format
cat <<EOF > ./vault/policy_admin.json
{
  "policy": "$(sed -e ':a;N;$!ba;s/\n/\\n/g' -e 's/"/\\"/g' vault/policy_admin.hcl)"
}
EOF

## create the admin policy
curl --silent \
  --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  --request PUT --data @./vault/policy_admin.json \
  http://$VAULT_ADDRESS/v1/sys/policies/acl/admin

curl --silent \
  --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  --request GET \
  http://$VAULT_ADDRESS/v1/sys/policies/acl/admin | jq
```

### Step 5: Create an `admin` role with the attached policy

```bash

## create the admin role with an attached policy
curl --silent \
  --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  --request POST \
  --data '{ "token_policies": "admin", "token_ttl": "1h", "token_max_ttl": "4h" }' \
  http://$VAULT_ADDRESS/v1/auth/approle/role/admin

## verify the role
curl --silent \
  --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  --request GET \
  http://$VAULT_ADDRESS/v1/auth/approle/role/admin | jq
```

### Step 6: Retrieve the admin token

From here, we'll want to get a admin token that we can use for the rest of the process:

```bash
VAULT_ADMIN_ROLE_ID=$(curl --silent \
  --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  http://$VAULT_ADDRESS/v1/auth/approle/role/admin/role-id | jq -r '.data.role_id'
)

VAULT_ADMIN_SECRET_ID=$(curl --silent \
  --header "X-Vault-Token: $VAULT_ROOT_TOKEN" \
  --request POST \
  http://$VAULT_ADDRESS/v1/auth/approle/role/admin/secret-id | jq -r '.data.secret_id'
)

export VAULT_ADMIN_TOKEN=$(curl --silent \
  --request POST \
  --data "{ \"role_id\": \"$VAULT_ADMIN_ROLE_ID\", \"secret_id\": \"$VAULT_ADMIN_SECRET_ID\" }" \
  http://$VAULT_ADDRESS/v1/auth/approle/login | jq -r '.auth.client_token'
)
```

### Step 7: Create a `dgraph` policy to access the secrets

```bash
## convert policies to json format
cat <<EOF > ./vault/policy_dgraph.json
{
  "policy": "$(sed -e ':a;N;$!ba;s/\n/\\n/g' -e 's/"/\\"/g' vault/policy_dgraph.hcl)"
}
EOF

## create the dgraph policy
curl --silent \
  --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
  --request PUT --data @./vault/policy_dgraph.json \
  http://$VAULT_ADDRESS/v1/sys/policies/acl/dgraph

## verify the policy
curl --silent \
  --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
  --request GET \
  http://$VAULT_ADDRESS/v1/sys/policies/acl/dgraph | jq
```


### Step 8: Create a `dgraph` role with the attached policy

```bash
## create the dgraph role with an attached policy
curl --silent \
 --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
 --request POST \
 --data '{ "token_policies": "dgraph", "token_ttl": "1h", "token_max_ttl": "4h" }' \
 http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph

## verify the role
curl --silent \
  --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" --request GET \
 http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph | jq
```

### Step 9: Save secrets using admin persona

This will save secrets for both [Encryption at Rest](https://dgraph.io/docs/enterprise-features/encryption-at-rest/) and [Access Control Lists](https://dgraph.io/docs/enterprise-features/access-control-lists/).

```bash
curl --silent \
  --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
  --request POST \
  --data @./vault/payload_alpha_secrets.json \
  http://$VAULT_ADDRESS/v1/secret/data/dgraph/alpha | jq
```

**NOTE**: When updating K/V Version 2 secrets, be sure to increment the `options.cas` value to increase the version.  For example, if updating the `enc_key` value to 32-bits, you would update `./vault/payload_alpha.secrests.json` to look like the following:
```json
{
  "options": {
    "cas": 1
  },
  "data": {
    "enc_key": "12345678901234567890123456789012",
    "hmac_secret_file": "12345678901234567890123456789012"
  }
}
```

### Step 10: Retrieve the dgraph token and save credentials

```bash
VAULT_DGRAPH_ROLE_ID=$(curl --silent \
  --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
  http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph/role-id | jq -r '.data.role_id'
)

VAULT_DGRAPH_SECRET_ID=$(curl --silent \
  --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
  --request POST \
  http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph/secret-id | jq -r '.data.secret_id'
)

export VAULT_DGRAPH_TOKEN=$(curl --silent \
  --request POST \
  --data "{ \"role_id\": \"$VAULT_DGRAPH_ROLE_ID\", \"secret_id\": \"$VAULT_DGRAPH_SECRET_ID\" }" \
  http://$VAULT_ADDRESS/v1/auth/approle/login | jq -r '.auth.client_token'
)
```

Also, we want to save the role-id and secret-id for the Dgraph Alpha server.

```bash
echo $VAULT_DGRAPH_ROLE_ID > ./vault/role_id
echo $VAULT_DGRAPH_SECRET_ID > ./vault/secret_id
```

### Step 11: Verify secrets access using app persona

```bash
curl --silent \
  --header "X-Vault-Token: $VAULT_DGRAPH_TOKEN" \
  --request GET \
  http://$VAULT_ADDRESS/v1/secret/data/dgraph/alpha | jq
```

### Step 12: Launch Dgraph

```bash
export DGRAPH_VERSION="<desired-dgraph-version>" # default 'latest'
docker-compose up --detach
```

You can verify encryption features are enabled with:

```bash
curl localhost:8080/health | jq -r '.[].ee_features | .[]' | sed 's/^/* /'
```

## Using Hashicorp Vault CIDR List for Authentication

As an alternative, you can restrict access to a limited range of IP addresses and disable the requirement for a `secret-id`.  In this scenario, we will set `bind_seccret_id` to `false`, and supply a list of IP addresses ranges for the `bound_cidr_list` key.

Only two steps will need to be changed, but otherwise the other steps are the same:

### Step 8: Create a `dgraph` role using `bound_cidr_list`

```bash
## create the dgraph role with an attached policy
curl --silent \
 --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
 --request POST \
 --data '{
"token_policies": "dgraph",
"token_ttl": "1h",
"token_max_ttl": "4h",
"bind_secret_id": false,
"bound_cidr_list": ["10.0.0.0/8","172.0.0.0/8","192.168.0.0/16", "127.0.0.1/32"]
}' \
 http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph

## verify the role
curl --silent \
  --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" --request GET \
 http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph | jq
```

### Step 10: Retrieve the dgraph token using only the `role-id`

```bash
VAULT_DGRAPH_ROLE_ID=$(curl --silent \
  --header "X-Vault-Token: $VAULT_ADMIN_TOKEN" \
  http://$VAULT_ADDRESS/v1/auth/approle/role/dgraph/role-id | jq -r '.data.role_id'
)

export VAULT_DGRAPH_TOKEN=$(curl --silent \
  --request POST \
  --data "{ \"role_id\": \"$VAULT_DGRAPH_ROLE_ID\" }" \
  http://$VAULT_ADDRESS/v1/auth/approle/login | jq -r '.auth.client_token'
)
```

Also, we want to save only the `role-id` for the Dgraph Alpha server.

```bash
echo $VAULT_DGRAPH_ROLE_ID > ./vault/role_id
```
