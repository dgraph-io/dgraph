#!/usr/bin/env bash

main() {
  get_secrets

  case $1 in
    minio_env)
      create_minio_env
      ;;
    minio_chart)
      create_minio_secrets
      ;;
    dgraph_chart)
      create_dgraph_secrets
      ;;
  esac
}

get_secrets() {
  MY_STORAGE_ACCT=${MY_STORAGE_ACCT:-""}
  MY_RESOURCE_GROUP=${MY_RESOURCE_GROUP:=""}

  if [[ -z "${MY_STORAGE_ACCT}" ]]; then
    printf "ERROR: The env var of 'MY_STORAGE_ACCT' was not defined. Exiting\n" 1>&2
    exit 1
  fi

  if [[ -z "${MY_RESOURCE_GROUP}" ]]; then
    printf "ERROR: The env var of 'MY_RESOURCE_GROUP' was not defined. Exiting\n" 1>&2
    exit 1
  fi

  CONN_STR=$(az storage account show-connection-string \
      --name "${MY_STORAGE_ACCT}" \
      --resource-group "${MY_RESOURCE_GROUP}" \
       | jq .connectionString -r
  )

  export MINIO_SECRET_KEY=$(grep -oP '(?<=AccountKey=).*' <<< $CONN_STR)
  export MINIO_ACCESS_KEY=$(grep -oP '(?<=AccountName=)[^;]*' <<< $CONN_STR)
}

create_minio_env() {
  cat <<-EOF > ../minio.env
MINIO_SECRET_KEY=$(grep -oP '(?<=AccountKey=).*' <<< $CONN_STR)
MINIO_ACCESS_KEY=$(grep -oP '(?<=AccountName=)[^;]*' <<< $CONN_STR)
EOF
}

create_minio_secrets() {
  cat <<-EOF > ../charts/minio_secrets.yaml
accessKey: ${MINIO_ACCESS_KEY}
secretKey: ${MINIO_SECRET_KEY}
EOF
}

create_dgraph_secrets() {
  cat <<-EOF > ../charts/dgraph_secrets.yaml
backups:
  keys:
    minio:
      access: ${MINIO_ACCESS_KEY}
      secret: ${MINIO_SECRET_KEY}
EOF
}

main $@
