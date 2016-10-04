package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

type defconf struct {
	N uint64
	K uint64 `json:"k"`
}

type config struct {
	// To preserve ordering we have a slice of maps here.
	Predicates []map[uint64]string `json:"predicates"`
	Default    defconf             `json:"default"`
}

type predMeta struct {
	val        string
	exactMatch bool
	gid        uint64
}

var predGroup []predMeta

var groupConfig config

func parsePredicates() {
	for _, m := range groupConfig.Predicates {
		for groupId, preds := range m {
			preds := strings.Split(preds, ",")
			x.Assertf(len(preds) > 0, "Length of predicates in config should be > 0")
			for _, pred := range preds {
				meta := predMeta{
					val: pred,
					gid: groupId,
				}
				if strings.HasSuffix(pred, "*") {
					meta.val = strings.TrimSuffix(meta.val, "*")
				} else {
					meta.exactMatch = true
				}
				predGroup = append(predGroup, meta)
			}
		}
	}
}

func ParseGroupConfig(file string) {
	cf, err := os.Open(file)
	x.Check(err)
	x.Check(json.NewDecoder(cf).Decode(&groupConfig))
	fmt.Println(groupConfig)
	x.Assertf(groupConfig.Default.N > 0, "N in config should be > 0")
	parsePredicates()
}

func fpGroup(pred string) uint64 {
	return farm.Fingerprint64([]byte(pred))%groupConfig.Default.N + groupConfig.Default.K
}

func group(pred string) uint64 {
	for _, meta := range predGroup {
		if meta.exactMatch && meta.val == pred {
			return meta.gid
		}
		if !meta.exactMatch && strings.HasPrefix(pred, meta.val) {
			return meta.gid
		}
	}
	return fpGroup(pred)
}
