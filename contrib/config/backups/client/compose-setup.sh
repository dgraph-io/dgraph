#!/usr/bin/env bash
######
## compose-setup.sh - configure a docker compose configuration and generate
##   private certs/keys using `dgraph cert` command.
##
##   This will also fetch an explicit Dgraph version that is tagged as `latest`
##   online if DGRAPH_VERSION environment variable is not specified.
##
##   This can be used to setup an environment that can be used explore Dgraph
##   backup functionality for operators
##########################

######
# main - runs the script
##########################
main() {
  parse_command $@
  config_compose
  create_certs
}

######
# usage - print friendly usage statement
##########################
usage() {
  cat <<-USAGE 1>&2
Setup Docker Compose Environment

Usage:
  $0 [FLAGS] --location [LOCATION]

Flags:
 -j, --acl                    Enable Access Control List
 -t, --auth_token             Enable auth token
 -e, --enc                    Enable Encryption
 -k, --tls                    Enable TLS
 -c, --tls_client_auth string Set TLS Auth String (default VERIFYIFGIVEN)
 -m, --make_tls_cert          Create TLS Certificates and Key
 -v, --dgraph_version         Set Dgraph Version
 -d, --debug                  Enable debug in output
 -h, --help                   Help for $0

USAGE
}

######
# get_grep - find grep that supports look-ahead/behind regex
##########################
get_grep() {
 unset GREP_CMD

 ## Check for GNU grep compatibility
 if ! grep --version | head -1 | fgrep -q GNU; then
   local SYSTEM="$(uname -s)"
   if [[ "${SYSTEM,,}" == "freebsd" ]]; then
     ## Check FreeBSD install location
     if [[ -f "/usr/local/bin/grep" ]]; then
        GREP_CMD="/usr/local/bin/grep"
     else
       ## Save FreeBSD Instructions
       local MESSAGE="On FreeBSD, compatible grep can be installed with 'sudo pkg install gnugrep'"
     fi
   elif [[ "${SYSTEM,,}" == "darwin" ]]; then
     ## Check HomeBrew install location
     if [[ -f "/usr/local/opt/grep/libexec/gnubin/grep" ]]; then
        GREP_CMD="/usr/local/opt/grep/libexec/gnubin/grep"
     ## Check MacPorts install location
     elif [[ -f "/opt/local/bin/grep" ]]; then
        GREP_CMD="/opt/local/bin/grep"
     else
        ## Save MacPorts or HomeBrew Instructions
        if command -v brew > /dev/null; then
          local MESSAGE="On macOS, gnu-grep can be installed with 'brew install grep'\n"
        elif command -v port > /dev/null; then
          local MESSAGE="On macOS, grep can be installed with 'sudo port install grep'\n"
        fi
     fi
   fi
 else
   GREP_CMD="$(command -v grep)"
 fi

 ## Error if no suitable grep command found
 if [[ -z $GREP_CMD ]]; then
   printf "ERROR: GNU grep not found.  Please install GNU compatible 'grep'\n\n%s" "$MESSAGE" 1>&2
   exit 1
 fi
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
    $GETOPT_CMD -o jtdhekmc:v: \
    --long acl,auth_token,enc,tls,make_tls_cert,tls_client_auth:,dgraph_version:,debug,help \
    -n 'compose-setup.sh' -- "$@"
  )
  if [ $? != 0 ] ; then usage; exit 1 ; fi
  eval set -- "$PARSED_ARGUMENTS"

  ## Defaults
  DEBUG="false"
  ACL_ENABLED="false"
  TOKEN_ENABLED="false"
  ENC_ENABLED="false"
  TLS_ENABLED="false"
  TLS_CLIENT_AUTH="VERIFYIFGIVEN"
  TLS_MAKE_CERTS="false"

  ## Process Agurments
  while true; do
    case "$1" in
      -j | --acl) ACL_ENABLED="true"; shift ;;
      -t | --auth_token) TOKEN_ENABLED=true; shift ;;
      -d | --debug) DEBUG="true"; shift ;;
      -h | --help) usage; exit;;
      -e | --enc) ENC_ENABLED="true"; shift ;;
      -k | --tls) TLS_ENABLED="true"; shift  ;;
      -m | --make_tls_cert) TLS_MAKE_CERTS="true"; shift;;
      -c | --tls_client_auth) TLS_CLIENT_AUTH="$2"; shift 2;;
      -v | --dgraph_version) DGRAPH_VERSION="$2"; shift 2;;
      --) shift; break ;;
      *) break ;;
    esac
  done

  ## Set DGRAPH_VERSION to latest if it is not set yet
  [[ -z $DGRAPH_VERSION ]] && get_grep && DGRAPH_VERSION=$(curl -s https://get.dgraph.io/latest | $GREP_CMD -oP '(?<=tag_name":")[^"]*')
}

######
# create_certs - creates cert and keys
##########################
create_certs() {
  command -v docker > /dev/null || \
    { echo "[ERROR]: 'docker' command not not found" 1>&2; exit 1; }
  docker version > /dev/null || \
    { echo "[ERROR]: docker not accessible for '$USER'" 1>&2; exit 1; }

  if [[ "$TLS_MAKE_CERTS" == "true" ]]; then
    [[ -z $DGRAPH_VERSION ]] && { echo "[ERROR]: 'DGRAPH_VERSION' not set. Aborting." 1>&2; exit 1; }
    rm --force $PWD/data/tls/*.{crt,key}
    docker run \
      --tty \
      --volume $PWD/data/tls:/tls dgraph/dgraph:$DGRAPH_VERSION \
      dgraph cert --dir /tls --client backupuser --nodes "localhost,alpha1,zero1" --duration 365
  fi
}

######
# config_compose - configures .env and data/config/config.tml
##########################
config_compose() {
  if [[ $DEBUG == "true" ]]; then
    set -ex
  else
    set -e
  fi

  CFGPATH="./data/config"
  mkdir -p ./data/config
  [[ -f $CFGPATH/config.toml ]] && rm $CFGPATH/config.toml
  touch $CFGPATH/config.toml

  ## configure defaults
  echo "whitelist = '10.0.0.0/8,172.16.0.0/12,192.168.0.0/16'" >> "$CFGPATH/config.toml"
  echo "lru_mb = 1024" >> "$CFGPATH/config.toml"

  ## configure if user specifies
  [[ $ACL_ENABLED == "true" ]] && \
    echo "--acl \"secret-file=/dgraph/acl/hmac_secret_file;\"" >> "$CFGPATH/config.toml"
  [[ $TOKEN_ENABLED == "true" ]] && \
    echo "auth_token = '$(cat ./data/token/auth_token_file)'" >> "$CFGPATH/config.toml"
  [[ $ENC_ENABLED == "true" ]] && \
    echo "--encryption \"key-file=/dgraph/enc/enc_key_file;\"" >> "$CFGPATH/config.toml"
  [[ $TLS_ENABLED == "true" ]] &&
    cat <<-TLS_CONFIG >> $CFGPATH/config.toml
tls_client_auth = '$TLS_CLIENT_AUTH'
TLS_CONFIG

  ## configure dgraph version
  echo "DGRAPH_VERSION=$DGRAPH_VERSION" > .env
  cp *backup*.sh data
}

main $@
