go build . && ./server --instanceIdx 0 --mutations ~/dgraph/m0 --port "8080" --postings ~/dgraph/p0 --workers ":12345,:12346,:12347" --uids ~/dgraph/u0 --workerport ":12345" &
go build . && ./server --instanceIdx 1 --mutations ~/dgraph/m1 --port "8081" --postings ~/dgraph/p1 --workers ":12345,:12346,:12347" --uids ~/dgraph/u1 --workerport ":12346" &
go build . && ./server --instanceIdx 2 --mutations ~/dgraph/m2 --port "8082" --postings ~/dgraph/p2 --workers ":12345,:12346,:12347" --uids ~/dgraph/u2 --workerport ":12347" &
