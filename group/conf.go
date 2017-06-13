/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package group

import (
	"bufio"
	"fmt"
	"io"
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
	n    uint32
	k    uint32
	pred []predMeta
}

var groupConfig config

func parsePredicates(groupId uint32, p string) error {
	preds := strings.Split(p, ",")
	x.AssertTruef(len(preds) > 0, "Length of predicates in config should be > 0")

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
	return nil
}

func parseDefaultConfig(l string) (uint32, error) {
	// If we have already seen a default config line, and n has a value then we
	// log.Fatal.
	if groupConfig.n != 0 {
		return 0, fmt.Errorf("Default config can only be defined once: %v", l)
	}
	l = strings.TrimSpace(l)
	conf := strings.Split(l, " ")
	// + in (fp % n + k) is optional.
	if !(len(conf) == 5 || len(conf) == 3) || conf[0] != "fp" || conf[1] != "%" {
		return 0, fmt.Errorf("Default config format should be like: %v", "default: fp % n + k")
	}

	var err error
	var n uint64
	n, err = strconv.ParseUint(conf[2], 10, 32)
	x.Check(err)
	groupConfig.n = uint32(n)
	x.AssertTrue(groupConfig.n != 0)
	if len(conf) == 5 {
		if conf[3] != "+" {
			return 0, fmt.Errorf("Default config format should be like: %v", "default: fp % n + k")
		}
		n, err = strconv.ParseUint(conf[4], 10, 32)
		groupConfig.k = uint32(n)
		x.Check(err)
	}
	if groupConfig.k == 0 {
		return 0, fmt.Errorf(`k in fp % n + k should be greater than zero.`)
	}
	return groupConfig.k, nil
}

// ParseConfig parses a group config provided by reader.
func ParseConfig(r io.Reader) error {
	groupConfig = config{}
	scanner := bufio.NewScanner(r)
	// To keep track of last groupId seen across lines. If we the groups ids are
	// not sequential, we log.Fatal.
	var curGroupId uint32 = 1
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
			return fmt.Errorf("Incorrect format for config line: %v", l)
		}
		if c[0] == "default" {
			seenDefault = true
			k, err := parseDefaultConfig(c[1])
			if err != nil {
				return err
			}
			if k == 0 {
				continue
			}
			if curGroupId != 0 && k > curGroupId {
				return fmt.Errorf("k in (fp mod N + k) should be <= the last groupno %v.",
					curGroupId)
			}
		} else {
			// There shouldn't be a line after the default config line.
			if seenDefault {
				return fmt.Errorf("Default config should be specified as the last line. Found %v",
					l)
			}
			groupId, err := strconv.ParseUint(c[0], 10, 32)
			x.Check(err)
			if groupId == 0 {
				return fmt.Errorf("Group ids should be greater than zero. Instead set to 0 for predicates: %v", c[1])
			}
			if curGroupId != uint32(groupId) {
				return fmt.Errorf("Group ids should be sequential and should start from 1. "+
					"Found %v, should have been %v", groupId, curGroupId)
			}
			curGroupId++
			err = parsePredicates(uint32(groupId), c[1])
			if err != nil {
				return err
			}
		}
	}
	x.Check(scanner.Err())
	return nil
}

// ParseGroupConfig parses the config file and stores the predicate <-> group map.
func ParseGroupConfig(file string) error {
	cf, err := os.Open(file)
	if os.IsNotExist(err) {
		groupConfig.n = 1
		groupConfig.k = 1
		return nil
	}
	x.Check(err)
	if err = ParseConfig(cf); err != nil {
		return err
	}
	if groupConfig.n == 0 {
		return fmt.Errorf("Cant take modulo 0.")
	}
	return nil
}

func fpGroup(pred string) uint32 {
	if groupConfig.n == 1 {
		return groupConfig.k
	}

	return farm.Fingerprint32([]byte(pred))%groupConfig.n + groupConfig.k
}

func BelongsTo(pred string) uint32 {
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
