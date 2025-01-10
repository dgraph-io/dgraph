#!/bin/bash
# This script modifies the pb.pb.go file to replace certain fields and methods
# to use the custom data type `Sensitive`.
#
# We are doing this because google.golang.org/protobuf does not support custom data types,
# and we have sensitive data that we do not want to expose in logs.
# To address this, we defined a `Sensitive` data type to handle such cases.
#
# This patch script applies the necessary changes to pb.pb.go.
PB_GEN_FILE="./pb/pb.pb.go"

sed -i 's/SessionToken string/SessionToken Sensitive/' "${PB_GEN_FILE}"
sed -i 's/SecretKey    string/SecretKey    Sensitive/' "${PB_GEN_FILE}"

sed -i 's/GetSessionToken() string {/GetSessionToken() Sensitive {/' "${PB_GEN_FILE}"
sed -i 's/GetSecretKey() string/GetSecretKey() Sensitive/' "${PB_GEN_FILE}"

echo "Patches applied successfully."
