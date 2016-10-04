package worker

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

type predMeta struct {
	val        string
	exactMatch bool
	gid        uint32
}

type config struct {
	n    uint64
	k    uint64
	pred []predMeta
}

var groupConfig config

func parsePredicates(groupId uint32, p string) {
	preds := strings.Split(p, ",")
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

func parseDefaultConfig(l string) uint64 {
	// If we have already seen a default config line, and n has a value then we
	// log.Fatal.
	if groupConfig.n != 0 {
		x.Check(fmt.Errorf("Default config can only be defined once: %v", l))
	}
	l = strings.TrimSpace(l)
	conf := strings.Split(l, " ")
	// + in (fp % n + k) is optional.
	fmt.Println(len(conf))
	if !(len(conf) == 5 || len(conf) == 3) || conf[0] != "fp" || conf[1] != "%" {
		x.Check(fmt.Errorf("Default config format should be like: %v", "default: fp % n + k"))
	}

	var err error
	groupConfig.n, err = strconv.ParseUint(conf[2], 10, 64)
	x.Check(err)
	if len(conf) == 5 {
		groupConfig.k, err = strconv.ParseUint(conf[4], 10, 64)
		x.Check(err)
	}
	return groupConfig.k
}

func parseConfig(f *os.File) {
	scanner := bufio.NewScanner(f)
	// To keep track of last groupId seen across lines. If we the groups ids are
	// not sequential, we log.Fatal.
	var currGroupId uint64
	// If after seeing line with default config, we see other lines, we log.Fatal.
	// Default config should be specified as the last line, so that we can check
	// accurately that k in (fp % N + k) generates consecutive groups.
	seenDefault := false
	for scanner.Scan() {
		l := scanner.Text()
		// Skip empty lines and comments.
		if l == "" || strings.HasPrefix(l, "//") {
			continue
		}
		c := strings.Split(l, ":")
		if len(c) < 2 {
			x.Check(fmt.Errorf("Incorrect format for config line: %v", l))
		}
		if c[0] == "default" {
			seenDefault = true
			k := parseDefaultConfig(c[1])
			if k == 0 {
				continue
			}
			x.Assertf(k <= currGroupId, "k in (fp mod N + k) should be <= the last groupno %v.",
				currGroupId)
		} else {
			// There shouldn't be a line after the default config line.
			if seenDefault {
				log.Fatalf("Default config should be specified as the last line. Found %v",
					l)
			}
			groupId, err := strconv.ParseUint(c[0], 10, 64)
			x.Check(err)
			x.Assertf(currGroupId == groupId,
				"Group ids should be sequential and should start from 0: %v", groupId)
			currGroupId++
			parsePredicates(uint32(groupId), c[1])
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

func fpGroup(pred string) uint32 {
	return farm.Fingerprint32([]byte(pred))%uint32(groupConfig.n) + uint32(groupConfig.k)
}

func group(pred string) uint32 {
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
