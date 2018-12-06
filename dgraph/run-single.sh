md5sum ~/go/bin/dgraph; go build . && go install . && md5sum dgraph ~/go/bin/dgraph
docker-compose -f docker-compose-single.yml down && docker-compose -f docker-compose-single.yml up --force-recreate --remove-orphans
