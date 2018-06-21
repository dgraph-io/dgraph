#!/bin/bash

set -e

bash contrib/scripts/loader.sh $1
bash contrib/scripts/transactions.sh $1
