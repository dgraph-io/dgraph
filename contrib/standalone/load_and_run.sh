#!/bin/bash

# fail if any error occurs
set -e

echo -e "\033[0;33m
Warning: This standalone version is meant for quickstart purposes only.
         It is NOT RECOMMENDED for production environments.\033[0;0m"

# For Dgraph versions v20.11 and older
export DGRAPH_ALPHA_WHITELIST=0.0.0.0/0
# For Dgraph versions v21.03 and newer
export DGRAPH_ALPHA_SECURITY='whitelist=0.0.0.0/0'

#!/bin/bash

# Define the folder
import_folder="import"
p_dir="p"
echo "Starting dgraph zero"
dgraph zero 1> /dev/null 2> zero.log &

# Check if folder A exists and contains .gz or .rdf files
if [[ ! -d "$p_dir" ]] ; then
    echo "No $p_dir directory found. Checking for data files in $import_folder folder."
    if [[ -d "$import_folder" ]]; then
        data_file=$(find "$import_folder" -maxdepth 1 -type f \( -name "*.gz" -o -name "*.rdf" \) | head -n 1)
        
        if [[ -n "$data_file" ]]; then
            # Get the full file name
            # data_file=$(basename "$file")
            
            # Get the name without the extension
            name_without_extension="${data_file%%.*}"
            schema_file="$name_without_extension.schema"
            graphql_file="$name_without_extension.graphql"
            
            echo "Found data file: $data_file"
            
            if [[ -f "$schema_file" ]]; then
                echo "Found dql schema file: $schema_file"
                sleep 2 # avoid error communicating with dgraph zero
                if [[ -f "$graphql_file" ]]; then
                    echo "Found graphql schema file: $graphql_file"
                    echo "Importing data using dgraph bulk loader"
                    echo "dgraph bulk -f $data_file -s $schema_file -g $graphql_file --map_shards=4 --reduce_shards=1"
                    dgraph bulk -f $data_file -s $schema_file -g $graphql_file --map_shards=4 --reduce_shards=1
                else
                    echo "Importing data using dgraph bulk loader"
                    echo "dgraph bulk -f $data_file -s $schema_file --map_shards=4 --reduce_shards=1"
                    dgraph bulk -f $data_file -s $schema_file --map_shards=4 --reduce_shards=1
                fi
                # copy generated p file to the root directory
                mv out/0/p p
            else
                echo "No schema file $schema_file found in $import_folder folder. Starting dgraph alpha without data."
            fi

        else
            echo "No .gz or .rdf files found in $import_folder folder. Starting dgraph alpha without data."
        fi
    else
        echo "No $import_folder folder found. Starting dgraph alpha without data."
    fi
else
    echo "$p_dir directory already exists."
fi
echo "Starting dgraph alpha"
dgraph alpha 1> /dev/null 2> alpha.log &

tail -f /dev/null