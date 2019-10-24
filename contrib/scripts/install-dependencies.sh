# Use this script if make install does not work because of dependency issues.
# Make sure to run the script from the dgraph repository root.

# Vendor opencensus.
rm -rf vendor/go.opencensus.io/
govendor fetch go.opencensus.io/...@v0.19.2
# Vendor prometheus.
rm -rf vendor/github.com/prometheus/
govendor fetch github.com/prometheus/client_golang/prometheus/...@v0.9.2
# Vendor gRPC.
govendor fetch google.golang.org/grpc/...@v1.13.0
# Vendor dgo (latest version before API changes).
govendor fetch github.com/dgraph-io/dgo...@v1.0.0

