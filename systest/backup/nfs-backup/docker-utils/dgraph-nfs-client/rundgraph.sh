echo "Starting NFS...............Setup"
/usr/local/bin/rpc_setup.sh /dgraph-data/backup 2>&1 & 
echo " Starting servers Dgraph $1"
echo $1
/gobin/dgraph  ${COVERAGE_OUTPUT}  alpha --my=$1:7080 --zero=$2 --logtostderr -v=2  --security "whitelist=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16;"  --tls "ca-cert=/dgraph-tls/ca.crt; server-cert=/dgraph-tls/node.crt; server-key=/dgraph-tls/node.key; internal-port=true; client-cert=/dgraph-tls/client.$1.crt; client-key=/dgraph-tls/client.$1.key;"  
