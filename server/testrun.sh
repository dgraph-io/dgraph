dir="/dgraph"
go build . && ./server --instanceIdx 0 --mutations $dir/m0 --port 8080 --postings $dir/p0 --workers ":12345,:12346,:12347" --uids $dir/uasync.final --workerport ":12345" &
go build . && ./server --instanceIdx 1 --mutations $dir/m1 --port 8082 --postings $dir/p1 --workers ":12345,:12346,:12347" --workerport ":12346" &
go build . && ./server --instanceIdx 2 --mutations $dir/m2 --port 8084 --postings $dir/p2 --workers ":12345,:12346,:12347" --workerport ":12347" &
