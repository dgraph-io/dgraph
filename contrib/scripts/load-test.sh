#!/bin/bash

set -e

source contrib/scripts/functions.sh

function finish {
  if [ $? -ne 0 ]; then
	  quit 0
  fi
}

trap finish EXIT

bash contrib/scripts/loader.sh $1

bash contrib/scripts/transactions.sh $1
