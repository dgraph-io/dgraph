#!/usr/bin/env bash

main() {
  parse_command
}

parse_command() {
  ## Check for GNU getopt
  if [[ "$(getopt --version)" =~ "--" ]]; then
    printf "ERROR: GNU getopt not found.  Please use GNU getopt" 1>&2
    if [[ "$(uname -s)" =~ "Darwin" ]]; then
      printf "On macOS with Homebrew (https://brew.sh/), gnu-getopt can be installed with:\n" 1>&2
      printf " brew install gnu-getopt\n" 1>&2
      printf ' export PATH="/usr/local/opt/gnu-getopt/bin:$PATH"\n' 1>&2
      printf "Info: https://formulae.brew.sh/formula/gnu-getopt" 1>&2
    fi
    exit 1
  fi

  TEMP=$(
    getopt -o dfma:l:t:b:u:p: --long debug,forcefull,miniosecure,alpha:,location:,subpath:,auth_token:,user:,password: \
           -n 'backup.sh' -- "$@"
  )

  if [ $? != 0 ] ; then echo "Terminating..." >&2 ; exit 1 ; fi

  # Note the quotes around `$TEMP': they are essential!
  eval set -- "$TEMP"

  DEBUG=false
  ALPHA_HOST=localhost
  BACKUP_DESTINATION=""
  SUB_PATH=""
  API_TYPE=""
  MINIO_SECURE=false
  AUTH_TOKEN=""
  ACCESS_TOKEN=""
  FORCE_FULL=false

  while true; do
    case "$1" in
      -d | --debug ) DEBUG=true; shift ;;
      -f | --forcefull ) FORCE_FULL=true; shift ;;
      -m | --miniosecure ) MINIO_SECURE=true; shift ;;
      -a | --alpha ) ALPHA_HOST="$2"; shift 2 ;;
      -l | --location ) BACKUP_DESTINATION="$2"; shift 2 ;;
      -b | --subpath ) SUB_PATH="$2"; shift 2 ;;
      -t | --auth_token ) AUTH_TOKEN="$2"; shift 2 ;;
      -u | --user ) USER=$USER; shift 2;;
      -p | --password ) PASSWORD=$PASSWORD; shift 2;;
      -- ) shift; break ;;
      * ) break ;;
    esac
  done

  cat <<TEST
DEBUG=$DEBUG
ALPHA_HOST=$ALPHA_HOST
BACKUP_DESTINATION=$BACKUP_DESTINATION
SUB_PATH=$SUB_PATH
API_TYPE=$API_TYPE
MINIO_SECURE=$MINIO_SECURE
AUTH_TOKEN=$AUTH_TOKEN
ACCESS_TOKEN=$ACCESS_TOKEN
FORCE_FULL=$FORCE_FULL
TEST

}

main $@
