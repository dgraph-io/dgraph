#!/usr/bin/env bash
######
## dgraph-backup.sh - general purpose shell script that can be used to
##  facilitate binary backups (an enterprise feature) with Dgraph.  This script
##  demonstrates how to use backups options available in either REST or
##  GraphQL API using the curl command.
##########################

######
# main - runs the script
##########################
main() {
  parse_command $@
  run_backup
}

######
# usage - print friendly usage statement
##########################
usage() {
  cat <<-USAGE 1>&2
Run Binary Backup

Usage:
  $0 [FLAGS] --location [LOCATION]

Flags:
 -a, --alpha string        Dgraph alpha HTTP/S server (default "localhost:8080")
 -i, --api_type            API Type of REST or GraphQL (default "GraphQL")
 -t, --auth_token string   The auth token passed to the server
 -d, --debug               Enable debug in output
 -f, --force_full          Force a full backup instead of an incremental backup.
 -h, --help                Help for $0
 -l, --location            Sets the source location URI (required).
     --minio_secure        Backups to MinIO will use https instead of http
 -p, --password            Password of the user if login is required.
     --subpath             Directory Path To Use to store backups, (default "dgraph_\$(date +%Y%m%d)")
     --tls_cacert filepath The CA Cert file used to verify server certificates. Required for enabling TLS.
     --tls_cert string     (optional) The Cert file provided by the client to the server.
     --tls_key string      (optional) The private key file provided by the client to the server.
 -u, --user                Username if login is required.

USAGE
}

######
# get_getopt - find GNU getopt or print error message
##########################
get_getopt() {
 unset GETOPT_CMD

 ## Check for GNU getopt compatibility
 if [[ "$(getopt --version)" =~ "--" ]]; then
   local SYSTEM="$(uname -s)"
   if [[ "${SYSTEM,,}" == "freebsd" ]]; then
     ## Check FreeBSD install location
     if [[ -f "/usr/local/bin/getopt" ]]; then
        GETOPT_CMD="/usr/local/bin/getopt"
     else
       ## Save FreeBSD Instructions
       local MESSAGE="On FreeBSD, compatible getopt can be installed with 'sudo pkg install getopt'"
     fi
   elif [[ "${SYSTEM,,}" == "darwin" ]]; then
     ## Check HomeBrew install location
     if [[ -f "/usr/local/opt/gnu-getopt/bin/getopt" ]]; then
        GETOPT_CMD="/usr/local/opt/gnu-getopt/bin/getopt"
     ## Check MacPorts install location
     elif [[ -f "/opt/local/bin/getopt" ]]; then
        GETOPT_CMD="/opt/local/bin/getopt"
     else
        ## Save MacPorts or HomeBrew Instructions
        if command -v brew > /dev/null; then
          local MESSAGE="On macOS, gnu-getopt can be installed with 'brew install gnu-getopt'\n"
        elif command -v port > /dev/null; then
          local MESSAGE="On macOS, getopt can be installed with 'sudo port install getopt'\n"
        fi
     fi
   fi
 else
   GETOPT_CMD="$(command -v getopt)"
 fi

 ## Error if no suitable getopt command found
 if [[ -z $GETOPT_CMD ]]; then
   printf "ERROR: GNU getopt not found.  Please install GNU compatible 'getopt'\n\n%s" "$MESSAGE" 1>&2
   exit 1
 fi
}

######
# parse_command - parse command line options using GNU getopt
##########################
parse_command() {
  get_getopt

  ## Parse Arguments with GNU getopt
  PARSED_ARGUMENTS=$(
    $GETOPT_CMD -o a:i:t:dfhl:p:u: \
    --long alpha:,api_type:,auth_token:,debug,force_full,help,location:,minio_secure,password:,subpath:,tls_cacert:,tls_cert:,tls_key:,user: \
    -n 'dgraph-backup.sh' -- "$@"
  )
  if [ $? != 0 ] ; then usage; exit 1 ; fi
  eval set -- "$PARSED_ARGUMENTS"

  ## Defaults
  DEBUG="false"
  ALPHA_HOST="localhost:8080"
  BACKUP_DESTINATION=""
  SUBPATH=dgraph_$(date +%Y%m%d)
  API_TYPE="graphql"
  MINIO_SECURE=false
  AUTH_TOKEN=""
  FORCE_FULL="false"

  ## Process Agurments
  while true; do
    case "$1" in
      -a | --alpha) ALPHA_HOST="$2"; shift 2 ;;
      -i | --api_type) API_TYPE=${2,,}; shift 2;;
      -t | --auth_token) AUTH_TOKEN="$2"; shift 2 ;;
      -d | --debug) DEBUG=true; shift ;;
      -f | --force_full) FORCE_FULL=true; shift ;;
      -h | --help) usage; exit;;
      -m | --minio_secure) MINIO_SECURE=true; shift ;;
      -l | --location) BACKUP_DESTINATION="$2"; shift 2 ;;
      -p | --password) ACL_PASSWORD="$2"; shift 2;;
      --subpath) SUBPATH="$2"; shift 2 ;;
      --tls_cacert) CACERT_PATH="$2"; shift 2 ;;
      --tls_cert) CLIENT_CERT_PATH="$2"; shift 2;;
      --tls_key) CLIENT_KEY_PATH="$2"; shift 2;;
      -u | --user) ACL_USER="$2"; shift 2;;
      --) shift; break ;;
      *) break ;;
    esac
  done

  ## Check required variable was set
  if [[ -z "$BACKUP_DESTINATION" ]]; then
    printf "ERROR: location was not specified!!\n\n"
    usage
    exit 1
  fi
}

######
# run_backup - using user specified options, execute backup
##########################
run_backup() {
  if [[ $DEBUG == "true" ]]; then
    set -ex
  else
    set -e
  fi

  [[ -f ./backup_helper.sh ]] || { echo "ERROR: Backup Script library (./backup_helper.sh) missing" 1>&2; exit 1; }
  source ./backup_helper.sh

  ## login if user was specified
  if ! [[ -z $ACL_USER ]]; then
    ACCESS_TOKEN=$(get_token $ACL_USER $ACL_PASSWORD $AUTH_TOKEN)
  fi

  ## perform backup with valid options set
  backup "$ACCESS_TOKEN" "$AUTH_TOKEN"
}

main $@
