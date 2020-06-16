#!/bin/bash

usage() { echo "Usage: $0 -e <Deployment url HTTP endpoint> -s <Graphql schema file path> [-t <Deployment Auth Token>]"; }

error() { echo $1; usage; exit 1; }
help() { usage; exit 0; }

while getopts ":he:s:t:" OPTION; do
    case ${OPTION} in
    h ) help
        ;;
    e ) url_endpoint=${OPTARG}
        ;;
    s ) schema_path=${OPTARG}
        ;;
    t ) auth_token=${OPTARG}
        ;;
    : ) error "-$OPTARG requires an argument"
        ;;
    ? ) error "Invalid argument"
        ;;
    esac
done
shift $((OPTIND -1))

if [[ -z ${url_endpoint} ]] || [[ -z ${schema_path} ]]
then
    error "Missing arguments"
fi

if [ ! -f ${schema_path} ]
then
    error "Schema file not found"
fi

curl_headers=(-H "Content-Type: application/json")
if [[ -n ${auth_token} ]]
then
    curl_headers+=(-H "Authorization: $auth_token")
fi

curl -vX POST "${curl_headers[@]}" ${url_endpoint}/admin -d@- <<EOGQL
{
    "query": "mutation updateGQLSchema(\$sch: String!) { updateGQLSchema(input: { set: { schema: \$sch } }) { gqlSchema { schema } } }",
    "variables": {
        "sch": "$(awk '{printf "%s\\n", $0}'  $schema_path)"
    }
}
EOGQL
