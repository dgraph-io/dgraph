#!/bin/bash

#!/bin/bash

# This file starts the Dgraph server, runs a simple mutation, does a query and checks the response.

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
        BUILD=$SRC/build
fi

ROCKSDBDIR=$BUILD/rocksdb-4.9.0

set -e

pushd $BUILD &> /dev/null
benchmark=$(pwd)/benchmarks/data
popd &> /dev/null

# build flags needed for rocksdb
export CGO_CFLAGS="-I${ROCKSDBDIR}/include"
export CGO_LDFLAGS="-L${ROCKSDBDIR}"
export LD_LIBRARY_PATH="${ROCKSDBDIR}:${LD_LIBRARY_PATH}"

pushd cmd/dgraph &> /dev/null
go build .
./dgraph --uids ~/dgraph/u0 --postings ~/dgraph/p0 --mutations ~/dgraph/m0 &

# Wait for server to start in the background.
until nc -z 127.0.0.1 8080;
do
        sleep 1
done

# Run the query.
curl http://localhost:8080/query -XPOST -d $'mutation {
        set {
            <alice-in-wonderland> <type> <novel> .
            <alice-in-wonderland> <character> <alice> .
            <alice-in-wonderland> <author> <lewis-carrol> .
            <alice-in-wonderland> <written-in> "1865" .
            <alice-in-wonderland> <name> "Alice in Wonderland" .
            <alice-in-wonderland> <sequel> <looking-glass> .
            <alice> <name> "Alice" .
            <alice> <name> "Алисия"@ru .
            <alice> <name> "Adélaïde"@fr .
            <lewis-carrol> <name> "Lewis Carroll" .
            <lewis-carrol> <born> "1832" .
            <lewis-carrol> <died> "1898" .
        }
}'

resp=$(curl http://localhost:8080/query -XPOST -d '
{
	me(_xid_: alice-in-wonderland) {
		type
		written-in
		name
		character {
                        name
			name.fr
			name.ru
		}
		author {
                        name
                        born
                        died
		}
	}
}')

check_val() {
        expected="$1"
        actual="$2"
        if [ "$expected" != "$actual" ];then
                echo -e "Expected value: $expected. Got: $actual"
                exit 1
        fi
}

authname=$(echo $resp | jq '.me.author.name')
check_val '"Lewis Carroll"' "$authname";

born=$(echo $resp | jq '.me.author.born')
check_val \"1832\" $born;

died=$(echo $resp | jq '.me.author.died')
check_val \"1898\" $died;

charname=$(echo $resp | jq '.me.character.name')
check_val \"Alice\" $charname;

name=$(echo $resp | jq '.me.name')
check_val '"Alice in Wonderland"' "$name";

written=$(echo $resp | jq --raw-output '.me | . ["written-in"]')
check_val "1865" $written;

GREEN='\033[0;32m'
NC='\033[0m'
printf "${GREEN}Query results matched expected values${NC}\n"

killall dgraph
popd &> /dev/null
