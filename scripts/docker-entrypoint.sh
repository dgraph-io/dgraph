#!/usr/bin/env bash

# Copyright 2019 ChainSafe Systems (ON) Corp.
# This file is part of gossamer.
#
# The gossamer library is free software: you can redistribute it and/or modify
# it under the terms of the GNU Lesser General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# The gossamer library is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU Lesser General Public License for more details.
#
# You should have received a copy of the GNU Lesser General Public License
# along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

mkdir -p ~/gossamer-dev;
BASE_PATH=~/gossamer-dev

set -euxo pipefail

if [ ! -f $BASE_PATH/genesis_created ]; then
	/usr/local/gossamer init --genesis=/gocode/src/github.com/ChainSafe/gossamer/chain/gssmr/genesis.json
	touch $BASE_PATH/genesis_created;
fi;

exec "$@"
