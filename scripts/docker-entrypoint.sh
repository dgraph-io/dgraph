#!/usr/bin/env bash
mkdir -p ~/gossamer-dev;
DATA_DIR=~/gossamer-dev

set -euxo pipefail

if [ ! -f $DATA_DIR/genesis_created ]; then
	/usr/local/gossamer init --genesis=/gocode/src/github.com/ChainSafe/gossamer/config/gssmr0.json
	touch $DATA_DIR/genesis_created;
fi;

exec "$@"
