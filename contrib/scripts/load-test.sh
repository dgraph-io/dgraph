#!/bin/bash

set -e

source contrib/scripts/functions.sh

function finish {
	quit 0
	rm -rf $1
}

trap finish EXIT

bash contrib/scripts/loader.sh $1

bash contrib/scripts/transactions.sh $1
