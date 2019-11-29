#!/usr/bin/env bash
mkdir -p ~/gossamer-dev;
DATA_DIR=~/gossamer-dev


if [ ! -f $DATA_DIR/genesis_created ]; then
	/usr/local/gossamer init --genesis=/gocode/src/github.com/ChainSafe/gossamer/genesis.json
	touch $DATA_DIR/genesis_created;
fi;

exec "$@"
