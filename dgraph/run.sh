md5sum ~/go/bin/dgraph; go build . && go install . && md5sum dgraph ~/go/bin/dgraph
docker-compose down; DATA=$HOME/dg docker-compose up --force-recreate --remove-orphans