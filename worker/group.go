package worker

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

type predMeta struct {
	val        string
	exactMatch bool
	gid        uint64
}

type config struct {
	n    uint64
	k    uint64
	pred []predMeta
}

var groupConfig config

func parsePredicates(groupId uint64, preds []string) {
	x.Assertf(len(preds) > 0, "Length of predicates in config should be > 0")
	for _, pred := range preds {
		pred = strings.TrimSpace(pred)
		meta := predMeta{
			val: pred,
			gid: groupId,
		}
		if strings.HasSuffix(pred, "*") {
			meta.val = strings.TrimSuffix(meta.val, "*")
		} else {
			meta.exactMatch = true
		}
		groupConfig.pred = append(groupConfig.pred, meta)
	}
}

func parseDefaultConfig(l string) {
	if groupConfig.n != 0 {
		x.Check(fmt.Errorf("Default config can only be defined once: %v", l))
	}
	l = strings.TrimSpace(l)
	conf := strings.Split(l, " ")
	if len(conf) != 5 {
		x.Check(fmt.Errorf("Default config format isn't correct: %v", l))
	}
	if conf[1] != "%" || conf[3] != "+" {
		x.Check(fmt.Errorf("Default config format should be like: %v", "default: attribute % n + k"))
	}

	var err error
	groupConfig.n, err = strconv.ParseUint(conf[2], 10, 64)
	x.Check(err)
	groupConfig.k, err = strconv.ParseUint(conf[4], 10, 64)
	x.Check(err)
}

func parsePredicateConfig(gid string, preds string) {
	groupId, err := strconv.ParseUint(gid, 10, 64)
	x.Check(err)
	p := strings.Split(preds, ",")
	parsePredicates(groupId, p)
}

func parseConfig(f *os.File) {
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		l := scanner.Text()
		if l == "" || strings.HasPrefix(l, "//") {
			continue
		}
		c := strings.Split(l, ":")
		if len(c) < 2 {
			x.Check(fmt.Errorf("Incorrect format for config line: %v", l))
		}
		if c[0] == "default" {
			parseDefaultConfig(c[1])
		} else {
			preds := strings.TrimSpace(c[1])
			parsePredicateConfig(c[0], preds)
		}
	}
	x.Check(scanner.Err())
}

// ParseGroupConfig parses the config file and stores the predicate <-> group map.
func ParseGroupConfig(file string) {
	cf, err := os.Open(file)
	x.Check(err)
	parseConfig(cf)
	x.Assertf(groupConfig.n > 0, "Cant take modulo 0.")
}

func fpGroup(pred string) uint64 {
	return farm.Fingerprint64([]byte(pred))%groupConfig.n + groupConfig.k
}

func group(pred string) uint64 {
	for _, meta := range groupConfig.pred {
		if meta.exactMatch && meta.val == pred {
			return meta.gid
		}
		if !meta.exactMatch && strings.HasPrefix(pred, meta.val) {
			return meta.gid
		}
	}
	return fpGroup(pred)
}
