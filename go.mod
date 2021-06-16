module github.com/dgraph-io/dgraph

go 1.12

// replace github.com/dgraph-io/badger/v3 => /home/mrjn/go/src/github.com/dgraph-io/badger
// replace github.com/dgraph-io/ristretto => /home/mrjn/go/src/github.com/dgraph-io/ristretto
// replace github.com/dgraph-io/sroar => /home/ash/go/src/github.com/dgraph-io/sroar

require (
	cloud.google.com/go/storage v1.15.0
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/Azure/azure-storage-blob-go v0.13.0
	github.com/DataDog/datadog-go v0.0.0-20190425163447-40bafcb5f6c1 // indirect
	github.com/DataDog/opencensus-go-exporter-datadog v0.0.0-20190503082300-0f32ad59ab08
	github.com/Masterminds/semver/v3 v3.1.0
	github.com/Microsoft/go-winio v0.4.15 // indirect
	github.com/OneOfOne/xxhash v1.2.5 // indirect
	github.com/Shopify/sarama v1.27.2
	github.com/blevesearch/bleve v1.0.13
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd
	github.com/dgraph-io/badger/v3 v3.0.0-20210527100413-b18860020f34
	github.com/dgraph-io/dgo/v210 v210.0.0-20210407152819-261d1c2a6987
	github.com/dgraph-io/gqlgen v0.13.2
	github.com/dgraph-io/gqlparser/v2 v2.2.0
	github.com/dgraph-io/graphql-transport-ws v0.0.0-20210511143556-2cef522f1f15
	github.com/dgraph-io/ristretto v0.0.4-0.20210504190834-0bf2acd73aa3
	github.com/dgraph-io/simdjson-go v0.3.0
	github.com/dgraph-io/sroar v0.0.0-20210604145002-865050cb7465
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/dgrijalva/jwt-go/v4 v4.0.0-preview1
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v1.13.1
	github.com/dustin/go-humanize v1.0.0
	github.com/getsentry/sentry-go v0.6.0
	github.com/go-sql-driver/mysql v0.0.0-20190330032241-c0f6b444ad8f
	github.com/gogo/protobuf v1.3.2
	github.com/golang/geo v0.0.0-20170810003146-31fb0106dc4a
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.3
	github.com/google/codesearch v1.0.0
	github.com/google/go-cmp v0.5.5
	github.com/google/uuid v1.1.2
	github.com/gorilla/websocket v1.4.2
	github.com/graph-gophers/graphql-go v0.0.0-20200309224638-dae41bde9ef9
	github.com/hashicorp/vault/api v1.0.4
	github.com/minio/minio-go/v6 v6.0.55
	github.com/mitchellh/panicwrap v1.0.0
	github.com/paulmach/go.geojson v0.0.0-20170327170536-40612a87147b
	github.com/pierrec/lz4 v2.6.0+incompatible // indirect
	github.com/pkg/errors v0.9.1
	github.com/pkg/profile v1.2.1
	github.com/prometheus/client_golang v0.9.3
	github.com/prometheus/common v0.4.1 // indirect
	github.com/prometheus/procfs v0.0.0-20190517135640-51af30a78b0e // indirect
	github.com/sergi/go-diff v1.1.0
	github.com/soheilhy/cmux v0.1.4
	github.com/spf13/cast v1.3.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.3
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.7.0
	github.com/tinylib/msgp v1.1.5 // indirect
	github.com/twpayne/go-geom v1.0.5
	github.com/xdg/scram v0.0.0-20180814205039-7eeb5667e42c
	go.etcd.io/etcd v0.0.0-20190228193606-a943ad0ee4c9
	go.opencensus.io v0.23.0
	go.uber.org/zap v1.16.0
	golang.org/x/crypto v0.0.0-20200820211705-5c72a883971a
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/net v0.0.0-20210510120150-4163338589ed
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210511113859-b0526f3d8744
	golang.org/x/text v0.3.6
	golang.org/x/tools v0.1.1
	google.golang.org/api v0.46.0
	google.golang.org/genproto v0.0.0-20210510173355-fb37daa5cd7a // indirect
	google.golang.org/grpc v1.37.1
	google.golang.org/grpc/examples v0.0.0-20210518002758-2713b77e8526 // indirect
	gopkg.in/DataDog/dd-trace-go.v1 v1.13.1 // indirect
	gopkg.in/square/go-jose.v2 v2.3.1
	gopkg.in/yaml.v2 v2.2.8
	src.techknowlogick.com/xgo v1.4.1-0.20210311222705-d25c33fcd864
)
