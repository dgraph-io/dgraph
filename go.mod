module github.com/dgraph-io/dgraph

go 1.18

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/DataDog/opencensus-go-exporter-datadog v0.0.0-20190503082300-0f32ad59ab08
	github.com/Masterminds/semver/v3 v3.1.0
	github.com/Shopify/sarama v1.27.2
	github.com/apache/thrift v0.13.0 // indirect
	github.com/blevesearch/bleve v1.0.13
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd
	github.com/dgraph-io/badger/v3 v3.2103.4
	github.com/dgraph-io/dgo/v210 v210.0.0-20210407152819-261d1c2a6987
	github.com/dgraph-io/gqlgen v0.13.2
	github.com/dgraph-io/gqlparser/v2 v2.2.1
	github.com/dgraph-io/graphql-transport-ws v0.0.0-20210511143556-2cef522f1f15
	github.com/dgraph-io/ristretto v0.1.1
	github.com/dgraph-io/simdjson-go v0.3.0
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/dgrijalva/jwt-go/v4 v4.0.0-preview1
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13
	github.com/dgryski/go-groupvarint v0.0.0-20190318181831-5ce5df8ca4e1
	github.com/docker/docker v1.13.1
	github.com/dustin/go-humanize v1.0.0
	github.com/getsentry/sentry-go v0.6.0
	github.com/go-sql-driver/mysql v0.0.0-20190330032241-c0f6b444ad8f
	github.com/gogo/protobuf v1.3.2
	github.com/golang/geo v0.0.0-20170810003146-31fb0106dc4a
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.3
	github.com/google/codesearch v1.0.0
	github.com/google/go-cmp v0.5.8
	github.com/google/uuid v1.0.0
	github.com/gorilla/websocket v1.4.2
	github.com/graph-gophers/graphql-go v0.0.0-20200309224638-dae41bde9ef9
	github.com/hashicorp/vault/api v1.0.4
	github.com/minio/minio-go/v6 v6.0.55
	github.com/mitchellh/panicwrap v1.0.0
	github.com/paulmach/go.geojson v0.0.0-20170327170536-40612a87147b
	github.com/pkg/errors v0.9.1
	github.com/pkg/profile v1.2.1
	github.com/prometheus/client_golang v0.9.3
	github.com/soheilhy/cmux v0.1.4
	github.com/spf13/cast v1.3.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.3
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.6.1
	github.com/twpayne/go-geom v1.0.5
	github.com/xdg/scram v0.0.0-20180814205039-7eeb5667e42c
	go.etcd.io/etcd v0.5.0-alpha.5.0.20190108173120-83c051b701d3
	go.opencensus.io v0.22.5
	go.uber.org/zap v1.16.0
	golang.org/x/crypto v0.1.0
	golang.org/x/exp v0.0.0-20221106115401-f9659909a136
	golang.org/x/net v0.1.0
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4
	golang.org/x/sys v0.1.0
	golang.org/x/text v0.4.0
	golang.org/x/tools v0.2.0
	google.golang.org/grpc v1.27.0
	gopkg.in/square/go-jose.v2 v2.3.1
	gopkg.in/yaml.v2 v2.3.0
	src.techknowlogick.com/xgo v1.4.1-0.20210311222705-d25c33fcd864
)

require (
	github.com/DataDog/datadog-go v0.0.0-20190425163447-40bafcb5f6c1 // indirect
	github.com/Microsoft/go-winio v0.4.15 // indirect
	github.com/OneOfOne/xxhash v1.2.5 // indirect
	github.com/agnivade/levenshtein v1.0.3 // indirect
	github.com/beorn7/perks v1.0.0 // indirect
	github.com/blevesearch/go-porterstemmer v1.0.3 // indirect
	github.com/blevesearch/segment v0.9.0 // indirect
	github.com/blevesearch/snowballstem v0.9.0 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.3.3 // indirect
	github.com/eapache/go-resiliency v1.2.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20180814174437-776d5712da21 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/fsnotify/fsnotify v1.4.7 // indirect
	github.com/golang/groupcache v0.0.0-20190702054246-869f871628b6 // indirect
	github.com/google/flatbuffers v1.12.1 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.1 // indirect
	github.com/hashicorp/go-multierror v1.0.0 // indirect
	github.com/hashicorp/go-retryablehttp v0.5.4 // indirect
	github.com/hashicorp/go-rootcerts v1.0.1 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2 // indirect
	github.com/hashicorp/go-uuid v1.0.2 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/vault/sdk v0.1.13 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/jcmturner/gofork v1.0.0 // indirect
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/klauspost/compress v1.12.3 // indirect
	github.com/klauspost/cpuid/v2 v2.0.3 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/minio/sha256-simd v0.1.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opentracing/opentracing-go v1.1.0 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/philhofer/fwd v1.0.0 // indirect
	github.com/pierrec/lz4 v2.6.0+incompatible // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4 // indirect
	github.com/prometheus/common v0.4.1 // indirect
	github.com/prometheus/procfs v0.0.0-20190517135640-51af30a78b0e // indirect
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/spf13/afero v1.1.2 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/subosito/gotenv v1.2.0 // indirect
	github.com/tinylib/msgp v1.1.0 // indirect
	github.com/willf/bitset v1.1.10 // indirect
	github.com/xdg/stringprep v1.0.0 // indirect
	go.uber.org/atomic v1.6.0 // indirect
	go.uber.org/multierr v1.5.0 // indirect
	golang.org/x/mod v0.6.0 // indirect
	golang.org/x/term v0.1.0 // indirect
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4 // indirect
	google.golang.org/api v0.13.0 // indirect
	google.golang.org/appengine v1.6.1 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.26.0 // indirect
	gopkg.in/DataDog/dd-trace-go.v1 v1.13.1 // indirect
	gopkg.in/ini.v1 v1.51.0 // indirect
	gopkg.in/jcmturner/aescts.v1 v1.0.1 // indirect
	gopkg.in/jcmturner/dnsutils.v1 v1.0.1 // indirect
	gopkg.in/jcmturner/gokrb5.v7 v7.5.0 // indirect
	gopkg.in/jcmturner/rpc.v1 v1.1.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // indirect
)
