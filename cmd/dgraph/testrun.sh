dir="$HOME/dgraph"
go build . && ./server --instanceIdx 0 --mutations $dir/m0 --port 8080 --postings $dir/p0_0_3 --workers ":12345,:12346" --uids $dir/uids_0_3 --workerport ":12345" --stw_ram_mb=6000 &
go build . && ./server --instanceIdx 1 --mutations $dir/m1 --port 8082 --postings $dir/p1_0_3 --workers ":12345,:12346" --workerport ":12346" &
