module github.com/dgraph-io/dgraph

go 1.19

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/DataDog/opencensus-go-exporter-datadog v0.0.0-20190503082300-0f32ad59ab08
	github.com/HdrHistogram/hdrhistogram-go v1.1.2
	github.com/IBM/sarama v1.41.0
	github.com/Masterminds/semver/v3 v3.1.0
	github.com/blevesearch/bleve/v2 v2.3.9
	github.com/dgraph-io/badger/v4 v4.2.0
	github.com/dgraph-io/dgo/v230 v230.0.1
	github.com/dgraph-io/gqlgen v0.13.2
	github.com/dgraph-io/gqlparser/v2 v2.2.1
	github.com/dgraph-io/graphql-transport-ws v0.0.0-20210511143556-2cef522f1f15
	github.com/dgraph-io/ristretto v0.1.1
	github.com/dgraph-io/simdjson-go v0.3.0
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13
	github.com/dgryski/go-groupvarint v0.0.0-20190318181831-5ce5df8ca4e1
	github.com/docker/docker v24.0.5+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/dustin/go-humanize v1.0.0
	github.com/getsentry/sentry-go v0.6.0
	github.com/go-sql-driver/mysql v0.0.0-20190330032241-c0f6b444ad8f
	github.com/gogo/protobuf v1.3.2
	github.com/golang-jwt/jwt/v5 v5.0.0
	github.com/golang/geo v0.0.0-20210211234256-740aa86cb551
	github.com/golang/glog v1.1.0
	github.com/golang/protobuf v1.5.3
	github.com/golang/snappy v0.0.4
	github.com/google/codesearch v1.0.0
	github.com/google/go-cmp v0.5.9
	github.com/google/uuid v1.3.0
	github.com/gorilla/websocket v1.4.2
	github.com/graph-gophers/graphql-go v1.3.0
	github.com/hashicorp/vault/api v1.0.4
	github.com/minio/minio-go/v6 v6.0.55
	github.com/mitchellh/panicwrap v1.0.0
	github.com/paulmach/go.geojson v0.0.0-20170327170536-40612a87147b
	github.com/pkg/errors v0.9.1
	github.com/pkg/profile v1.2.1
	github.com/prometheus/client_golang v1.14.0
	github.com/soheilhy/cmux v0.1.4
	github.com/spf13/cast v1.3.0
	github.com/spf13/cobra v1.7.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.8.4
	github.com/twpayne/go-geom v1.0.5
	github.com/xdg/scram v0.0.0-20180814205039-7eeb5667e42c
	go.etcd.io/etcd/raft/v3 v3.5.9
	go.opencensus.io v0.24.0
	go.uber.org/zap v1.16.0
	golang.org/x/crypto v0.12.0
	golang.org/x/net v0.14.0
	golang.org/x/sync v0.3.0
	golang.org/x/sys v0.11.0
	golang.org/x/term v0.11.0
	golang.org/x/text v0.12.0
	golang.org/x/tools v0.9.3
	google.golang.org/grpc v1.56.2
	google.golang.org/protobuf v1.31.0
	gopkg.in/square/go-jose.v2 v2.3.1
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/DataDog/datadog-go v3.5.0+incompatible // indirect
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/agnivade/levenshtein v1.0.3 // indirect
	github.com/apache/thrift v0.13.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.2.0 // indirect
	github.com/blevesearch/bleve_index_api v1.0.5 // indirect
	github.com/blevesearch/geo v0.1.17 // indirect
	github.com/blevesearch/go-porterstemmer v1.0.3 // indirect
	github.com/blevesearch/segment v0.9.1 // indirect
	github.com/blevesearch/snowballstem v0.9.0 // indirect
	github.com/blevesearch/upsidedown_store_api v1.0.2 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/eapache/go-resiliency v1.4.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20230731223053-c322873962e3 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/frankban/quicktest v1.10.2 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/flatbuffers v1.12.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.6.6 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/vault/sdk v0.1.13 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.16.7 // indirect
	github.com/klauspost/cpuid/v2 v2.0.3 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/minio/sha256-simd v0.1.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/philhofer/fwd v1.0.0 // indirect
	github.com/pierrec/lz4 v2.5.2+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.18 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.42.0 // indirect
	github.com/prometheus/procfs v0.10.1 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/rogpeppe/go-internal v1.6.1 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/spf13/afero v1.9.5 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/subosito/gotenv v1.4.2 // indirect
	github.com/tinylib/msgp v1.1.2 // indirect
	github.com/xdg/stringprep v1.0.3 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/mod v0.12.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/api v0.122.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230711160842-782d3b101e98 // indirect
	google.golang.org/grpc/examples v0.0.0-20230821201920-d51b3f41716d // indirect
	gopkg.in/DataDog/dd-trace-go.v1 v1.22.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gotest.tools/v3 v3.5.0 // indirect
)
