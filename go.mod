module github.com/dgraph-io/dgraph

go 1.12

// replace github.com/dgraph-io/ristretto => /home/mrjn/go/src/github.com/dgraph-io/ristretto

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/99designs/gqlgen v0.13.1-0.20200928230741-819e751c2416
	github.com/DataDog/datadog-go v0.0.0-20190425163447-40bafcb5f6c1 // indirect
	github.com/DataDog/opencensus-go-exporter-datadog v0.0.0-20190503082300-0f32ad59ab08
	github.com/DataDog/zstd v1.4.5 // indirect
	github.com/Masterminds/semver/v3 v3.1.0
	github.com/OneOfOne/xxhash v1.2.5 // indirect
	github.com/beorn7/perks v1.0.0 // indirect
	github.com/blevesearch/bleve v0.0.0-20181114232033-e1f5e6cdcd76
	github.com/blevesearch/go-porterstemmer v1.0.2 // indirect
	github.com/blevesearch/segment v0.0.0-20160915185041-762005e7a34f // indirect
	github.com/blevesearch/snowballstem v0.0.0-20180110192139-26b06a2c243d // indirect
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd
	github.com/dgraph-io/badger/v2 v2.0.1-rc1.0.20201009032841-801e9f26e1bb
	github.com/dgraph-io/dgo/v200 v200.0.0-20200805103119-a3544c464dd6
	github.com/dgraph-io/graphql-transport-ws v0.0.0-20200916064635-48589439591b
	github.com/dgraph-io/ristretto v0.0.4-0.20201007164332-9739cfa2564b
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/dgrijalva/jwt-go/v4 v4.0.0-preview1
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13
	github.com/dgryski/go-groupvarint v0.0.0-20190318181831-5ce5df8ca4e1
	github.com/dustin/go-humanize v1.0.0
	github.com/getsentry/sentry-go v0.6.0
	github.com/go-sql-driver/mysql v0.0.0-20190330032241-c0f6b444ad8f
	github.com/gogo/protobuf v1.3.1
	github.com/golang/geo v0.0.0-20170810003146-31fb0106dc4a
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.3.5
	github.com/golang/snappy v0.0.1
	github.com/google/codesearch v1.0.0
	github.com/google/go-cmp v0.5.0
	github.com/google/uuid v1.0.0
	github.com/gorilla/websocket v1.4.2
	github.com/graph-gophers/graphql-go v0.0.0-20200309224638-dae41bde9ef9
	github.com/graph-gophers/graphql-transport-ws v0.0.0-20190611222414-40c048432299 // indirect
	github.com/hashicorp/vault/api v1.0.4
	github.com/minio/minio-go/v6 v6.0.55
	github.com/mitchellh/panicwrap v1.0.0
	github.com/paulmach/go.geojson v0.0.0-20170327170536-40612a87147b
	github.com/philhofer/fwd v1.0.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/pkg/profile v1.2.1
	github.com/prometheus/client_golang v0.9.3-0.20190127221311-3c4408c8b829
	github.com/prometheus/common v0.4.1 // indirect
	github.com/prometheus/procfs v0.0.0-20190517135640-51af30a78b0e // indirect
	github.com/spf13/cast v1.3.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.3
	github.com/spf13/viper v1.3.2
	github.com/stretchr/testify v1.4.0
	github.com/tinylib/msgp v0.0.0-20190103190839-ade0ca4ace05 // indirect
	github.com/twpayne/go-geom v1.0.5
	github.com/vektah/gqlparser/v2 v2.1.0
	github.com/willf/bitset v0.0.0-20181014161241-71fa2377963f // indirect
	go.etcd.io/etcd v0.0.0-20190228193606-a943ad0ee4c9
	go.opencensus.io v0.21.0
	golang.org/x/crypto v0.0.0-20191011191535-87dc89f01550
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20200918174421-af09f7315aff
	golang.org/x/text v0.3.2
	google.golang.org/grpc v1.23.0
	gopkg.in/DataDog/dd-trace-go.v1 v1.13.1 // indirect
	gopkg.in/ini.v1 v1.48.0 // indirect
	gopkg.in/square/go-jose.v2 v2.3.1
	gopkg.in/yaml.v2 v2.2.4
)
