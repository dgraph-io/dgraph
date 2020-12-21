######
## backup_helper.sh - general purpose shell script library used to support
##  Dgraph binary backups enterprise feature.
##########################

######
# get_token_rest - get accessJWT token with REST command for Dgraph 1.x
##########################
get_token_rest() {
  JSON="{\"userid\": \"${USER}\", \"password\": \"${PASSWORD}\" }"
  RESULT=$(
    /usr/bin/curl --silent \
      "${HEADERS[@]}" \
      "${CERTOPTS[@]}" \
      --request POST \
      ${ALPHA_HOST}/login \
      --data "${JSON}"
  )

  if grep -q errors <<< "$RESULT"; then
    ERROR=$(grep -oP '(?<=message":")[^"]*' <<< $RESULT)
    echo "ERROR: $ERROR"
    return 1
  fi

  grep -oP '(?<=accessJWT":")[^"]*' <<< "$RESULT"

}

######
# get_token_graphql - get accessJWT token using GraphQL for Dgraph 20.03.1+
##########################
get_token_graphql() {
  GQL="{\"query\": \"mutation { login(userId: \\\"${USER}\\\" password: \\\"${PASSWORD}\\\") { response { accessJWT } } }\"}"
  RESULT=$(
    /usr/bin/curl --silent \
      "${HEADERS[@]}" \
      "${CERTOPTS[@]}" \
      --request POST \
      ${ALPHA_HOST}/admin \
      --data "${GQL}"
  )

  if grep -q errors <<< "$RESULT"; then
    ERROR=$(grep -oP '(?<=message":")[^"]*' <<< $RESULT)
    echo "ERROR: $ERROR"
    return 1
  fi

  grep -oP '(?<=accessJWT":")[^"]*' <<< "$RESULT"

}

######
# get_token - get accessJWT using GraphQL /admin or REST /login
#  params:
#    1: user (required)
#    2: password (required)
#  envvars:
#    ALPHA_HOST (default: localhost:8080) - dns name of dgraph alpha node
#    CACERT_PATH - path to dgraph root ca (e.g. ca.crt) if TLS is enabled
#    CLIENT_CERT_PATH - path to client cert (e.g. client.dgraphuser.crt) for client TLS
#    CLIENT_KEY_PATH - path to client cert (e.g. client.dgraphuser.key) for client TLS
##########################
get_token() {
  USER=${1}
  PASSWORD=${2}
  AUTH_TOKEN=${3:-""}
  CACERT_PATH=${CACERT_PATH:-""}
  CLIENT_CERT_PATH=${CLIENT_CERT_PATH:-""}
  CLIENT_KEY_PATH=${CLIENT_KEY_PATH:-""}

  ## user/password required for login
  if [[ -z "$USER" || -z "$PASSWORD" ]]; then
    return 1
  fi

  if [[ ! -z "$AUTH_TOKEN" ]]; then
    HEADERS+=('--header' "X-Dgraph-AuthToken: $AUTH_TOKEN")
  fi

  if [[ ! -z "$CACERT_PATH" ]]; then
    CERTOPTS+=('--cacert' "$CACERT_PATH")
    if [[ ! -z "$CLIENT_CERT_PATH" || ! -z "$CLIENT_KEY_PATH" ]]; then
      CERTOPTS+=(
        '--cert' "$CLIENT_CERT_PATH"
        '--key' "$CLIENT_KEY_PATH"
      )
    fi
    ALPHA_HOST=https://${ALPHA_HOST:-"localhost:8080"}
  else
    ALPHA_HOST=${ALPHA_HOST:-"localhost:8080"}
  fi

  API_TYPE=${API_TYPE:-"graphql"}
  if [[ "$API_TYPE" == "graphql" ]]; then
    HEADERS+=('--header' "Content-Type: application/json")
    get_token_graphql
  else
    get_token_rest
  fi
}

######
# backup - trigger binary backup GraphQL /admin or REST /login
#  params:
#    1: token (optional) - if ACL enabled pass token from get_token()
#  envvars:
#    BACKUP_DESTINATION (required) - filepath ("/path/to/backup"), s3://, or minio://
#    ALPHA_HOST (default: localhost:8080) - dns name of dgraph alpha node
#    MINIO_SECURE (default: false) - set to true if minio service supports https
#    FORCE_FULL (default: false) - set to true if forcing a full backup
#    CACERT_PATH - path to dgraph root ca (e.g. ca.crt) if TLS is enabled
#    CLIENT_CERT_PATH - path to client cert (e.g. client.dgraphuser.crt) for client TLS
#    CLIENT_KEY_PATH - path to client cert (e.g. client.dgraphuser.key) for client TLS
##########################
backup() {
  ACCESS_TOKEN=${1:-""}
  AUTH_TOKEN=${2:-""}
  CACERT_PATH=${CACERT_PATH:-""}
  CLIENT_CERT_PATH=${CLIENT_CERT_PATH:-""}
  CLIENT_KEY_PATH=${CLIENT_KEY_PATH:-""}

  API_TYPE=${API_TYPE:-"graphql"}

  MINIO_SECURE=${MINIO_SECURE:-"false"}
  FORCE_FULL=${FORCE_FULL:-"false"}

  [[ -z "$BACKUP_DESTINATION" ]] && \
    { echo "'BACKUP_DESTINATION' is not set. Exiting" >&2; return 1; }

  if [[ ! -z "$ACCESS_TOKEN" ]]; then
    HEADERS+=('--header' "X-Dgraph-AccessToken: $ACCESS_TOKEN")
  fi

  if [[ ! -z "$AUTH_TOKEN" ]]; then
    HEADERS+=('--header' "X-Dgraph-AuthToken: $AUTH_TOKEN")
  fi

  if [[ ! -z "$CACERT_PATH" ]]; then
    CERTOPTS+=('--cacert' "$CACERT_PATH")
    if [[ ! -z "$CLIENT_CERT_PATH" || ! -z "$CLIENT_KEY_PATH" ]]; then
      CERTOPTS+=(
        '--cert' "$CLIENT_CERT_PATH"
        '--key' "$CLIENT_KEY_PATH"
      )
    fi
    ALPHA_HOST=https://${ALPHA_HOST:-"localhost:8080"}
  else
    ALPHA_HOST=${ALPHA_HOST:-"localhost:8080"}
  fi

  ## Configure destination with date stamp folder
  BACKUP_DESTINATION="${BACKUP_DESTINATION}/${SUBPATH}"
  ## Configure Minio Configuration
  if [[ "$MINIO_SECURE" == "false" && "$BACKUP_DESTINATION" =~ ^minio ]]; then
      BACKUP_DESTINATION="${BACKUP_DESTINATION}?secure=false"
  fi

  ## Create date-stamped directory for file system
  if [[ ! "$BACKUP_DESTINATION" =~ ^minio|^s3 ]]; then
    ## Check destination directory exist
    if [[ -d ${BACKUP_DESTINATION%/*} ]]; then
      mkdir -p $BACKUP_DESTINATION
    else
      echo "Designated Backup Destination '${BACKUP_DESTINATION%/*}' does not exist. Aborting."
      return 1
    fi
  fi

  if [[ "$API_TYPE" == "graphql" ]]; then
    HEADERS+=('--header' "Content-Type: application/json")
    backup_graphql
  else
    backup_rest
  fi

}

######
# backup_rest - trigger backup using REST command for Dgraph 1.x
##########################
backup_rest() {
  URL_PATH="admin/backup?force_full=$FORCE_FULL"

  RESULT=$(/usr/bin/curl --silent \
    "${HEADERS[@]}" \
    "${CERTOPTS[@]}" \
    --request POST \
    ${ALPHA_HOST}/$URL_PATH \
    --data "destination=$BACKUP_DESTINATION"
  )

  if grep -q errors <<< "$RESULT"; then
    ERROR=$(grep -oP '(?<=message":")[^"]*' <<< $RESULT)
    MESSAGE="ERROR: $ERROR"
    if grep -q code <<< "$RESULT"; then
      CODE=$(grep -oP '(?<=code":")[^"]*' <<< $RESULT)
      echo "$MESSAGE REASON='$CODE'"
    fi
    return 1
  fi

  echo $RESULT

}

######
# backup_graphql - trigger backup using GraphQL for Dgraph 20.03.1+
##########################
backup_graphql() {
  GQL="{\"query\": \"mutation { backup(input: {destination: \\\"${BACKUP_DESTINATION}\\\" forceFull: $FORCE_FULL }) { response { message code } } }\"}"

  RESULT=$(/usr/bin/curl --silent \
    "${HEADERS[@]}" \
    "${CERTOPTS[@]}" \
    --request POST \
    ${ALPHA_HOST}/admin \
    --data "$GQL"
  )

  if grep -q errors <<< "$RESULT"; then
    ERROR=$(grep -oP '(?<=message":")[^"]*' <<< $RESULT)
    echo "ERROR: $ERROR"
    return 1
  fi

  echo $RESULT
}
