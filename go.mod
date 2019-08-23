module github.com/dgraph-io/dgraph

go 1.12

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/AndreasBriese/bbloom v0.0.0-20190306092124-e2d15f34fcf9
	github.com/DataDog/datadog-go v0.0.0-20190425163447-40bafcb5f6c1
	github.com/DataDog/opencensus-go-exporter-datadog v0.0.0-20190503082300-0f32ad59ab08
	github.com/DataDog/zstd v1.3.6-0.20190409195224-796139022798
	github.com/MakeNowJust/heredoc v0.0.0-20140704152643-1d91351acdc1
	github.com/Shopify/sarama v1.19.0
	github.com/apache/thrift v0.12.0
	github.com/beorn7/perks v1.0.0
	github.com/blevesearch/bleve v0.0.0-20181114232033-e1f5e6cdcd76
	github.com/blevesearch/go-porterstemmer v1.0.2
	github.com/blevesearch/segment v0.0.0-20160915185041-762005e7a34f
	github.com/blevesearch/snowballstem v0.0.0-20180110192139-26b06a2c243d
	github.com/cespare/xxhash v1.1.0
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd
	github.com/davecgh/go-spew v1.1.1
	github.com/dgraph-io/badger v0.0.0-20190809121831-9d7b751e85c9
	github.com/dgraph-io/dgo v1.0.0 // indirect
	github.com/dgraph-io/dgo/v2 v2.0.0-20190823041719-dd13326bfd54
	github.com/dgraph-io/ristretto v0.0.0-20190802090228-1146b50c9e72
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/dgryski/go-farm v0.0.0-20190423205320-6a90982ecee2
	github.com/dgryski/go-groupvarint v0.0.0-20190318181831-5ce5df8ca4e1
	github.com/dustin/go-humanize v1.0.0
	github.com/eapache/go-resiliency v1.1.0
	github.com/eapache/go-xerial-snappy v0.0.0-20180814174437-776d5712da21
	github.com/eapache/queue v1.1.0
	github.com/fsnotify/fsnotify v1.4.7
	github.com/go-ini/ini v1.39.0
	github.com/go-logfmt/logfmt v0.4.0 // indirect
	github.com/go-sql-driver/mysql v0.0.0-20190330032241-c0f6b444ad8f
	github.com/gogo/protobuf v1.2.0
	github.com/golang/geo v0.0.0-20170810003146-31fb0106dc4a
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.3.2
	github.com/golang/snappy v0.0.1
	github.com/google/codesearch v1.0.0
	github.com/google/uuid v1.0.0
	github.com/hashicorp/go-uuid v1.0.1
	github.com/hashicorp/golang-lru v0.5.0
	github.com/hashicorp/hcl v1.0.0
	github.com/jcmturner/gofork v1.0.0
	github.com/magiconair/properties v1.8.0
	github.com/matttproud/golang_protobuf_extensions v1.0.1
	github.com/minio/minio-go v0.0.0-20181109183348-774475480ffe
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mitchellh/mapstructure v1.1.2
	github.com/onsi/ginkgo v1.7.0 // indirect
	github.com/onsi/gomega v1.4.3 // indirect
	github.com/paulmach/go.geojson v0.0.0-20170327170536-40612a87147b
	github.com/pelletier/go-toml v1.2.0
	github.com/philhofer/fwd v1.0.0
	github.com/pierrec/lz4 v2.0.5+incompatible
	github.com/pkg/errors v0.8.1
	github.com/pkg/profile v1.2.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/client_golang v0.9.3-0.20190127221311-3c4408c8b829
	github.com/prometheus/client_model v0.0.0-20190129233127-fd36f4220a90
	github.com/prometheus/common v0.4.1
	github.com/prometheus/procfs v0.0.0-20190517135640-51af30a78b0e
	github.com/rcrowley/go-metrics v0.0.0-20181016184325-3113b8401b8a
	github.com/spf13/afero v1.1.2
	github.com/spf13/cast v1.3.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/jwalterweatherman v1.0.0
	github.com/spf13/pflag v1.0.3
	github.com/spf13/viper v1.3.2
	github.com/stretchr/testify v1.3.0
	github.com/tinylib/msgp v0.0.0-20190103190839-ade0ca4ace05
	github.com/twpayne/go-geom v0.0.0-20170317090630-6753ad11e46b
	github.com/willf/bitset v0.0.0-20181014161241-71fa2377963f
	go.etcd.io/etcd v0.0.0-20190228193606-a943ad0ee4c9
	go.opencensus.io v0.21.0
	golang.org/x/crypto v0.0.0-20190701094942-4def268fd1a4
	golang.org/x/net v0.0.0-20190620200207-3b0461eec859
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20190626221950-04f50cda93cb
	golang.org/x/text v0.3.0
	google.golang.org/api v0.3.2
	google.golang.org/genproto v0.0.0-20190516172635-bb713bdc0e52
	google.golang.org/grpc v1.23.0
	gopkg.in/DataDog/dd-trace-go.v1 v1.13.1
	gopkg.in/jcmturner/aescts.v1 v1.0.1
	gopkg.in/jcmturner/dnsutils.v1 v1.0.1
	gopkg.in/jcmturner/gokrb5.v7 v7.2.4
	gopkg.in/jcmturner/rpc.v1 v1.1.0
	gopkg.in/yaml.v2 v2.2.2
)

replace github.com/dgraph-io/dgo/v2 => ../dgo
