#!/usr/bin/env bash

# Purpose:
#  This builds a keypair and installs them into all availavble regions
# Background:
#  CloudFormation script requires keypair as parameter to be in existing region
# Requirements:
#  aws cli tools with profile (~/.aws/) configured
#

#####
# main
######
main() {
  parse_arguments
  # generate local private/public key
  generate_key
  # install keys into target regions
  seed_keys "$REGIONS"
}

#####
# parse_arguments - set global vars from command line args
######
parse_arguments() {
  REGIONS=${1:-$(aws ec2 describe-regions \
   --query 'Regions[].{Name:RegionName}' \
   --output text
  )}

  KEYPAIR=${2:-"dgraph-marketplace-cf-stack-key"}
  KEYPATH=${3:-"."}
  KEYNAME=${4:-"dgraph"}
}

#####
# seed_keys - install public key with keypair name into target regions
######
seed_keys() {
  local REGIONS="$1"

  for REGION in $REGIONS; do
    create_key_pair $REGION
  done
}

#####
# generate_key - generate private/public key pair if private key doesn't exist
######
generate_key() {
  if [[ ! -f $KEYPATH/$KEYNAME.pem ]]; then
    # Generate Key Pair
    openssl genrsa -out "$KEYPATH/$KEYNAME.pem" 4096
    openssl rsa -in "$KEYPATH/$KEYNAME.pem" -pubout > "$KEYPATH/$KEYNAME.pub"
    chmod 400 "$KEYPATH/$KEYNAME.pem"
  fi
}

#####
# create_key_pair - upload public key with key_pair name
######
create_key_pair() {
  local REGION=${1:-$(aws configure get region)}

  # Install Keys into Metadata
  echo "Creating KeyPair in $REGION"
  aws ec2 import-key-pair \
   --region $REGION \
   --key-name $KEYPAIR \
   --public-key-material "$(grep -v PUBLIC $KEYPATH/$KEYNAME.pub | tr -d '\n')"
}

main
