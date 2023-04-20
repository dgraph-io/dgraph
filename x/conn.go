/*
 * Copyright 2023-2023 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package x

import (
	"bytes"
	"fmt"
	"net"
	"strings"

	"github.com/golang/glog"
	"github.com/pkg/errors"
)

// Parses a comma-delimited list of IP addresses, IP ranges, CIDR blocks, or hostnames
// and returns a slice of []IPRange.
//
// e.g. "144.142.126.222:144.142.126.244,144.142.126.254,192.168.0.0/16,host.docker.internal"
func GetIPsFromString(str string) ([]IPRange, error) {
	if str == "" {
		return []IPRange{}, nil
	}

	var ipRanges []IPRange
	rangeStrings := strings.Split(str, ",")

	for _, s := range rangeStrings {
		isIPv6 := strings.Contains(s, "::")
		tuple := strings.Split(s, ":")
		switch {
		case isIPv6 || len(tuple) == 1:
			if !strings.Contains(s, "/") {
				// string is hostname like host.docker.internal,
				// or IPv4 address like 144.124.126.254,
				// or IPv6 address like fd03:b188:0f3c:9ec4::babe:face
				ipAddr := net.ParseIP(s)
				if ipAddr != nil {
					ipRanges = append(ipRanges, IPRange{Lower: ipAddr, Upper: ipAddr})
				} else {
					ipAddrs, err := net.LookupIP(s)
					if err != nil {
						return nil, errors.Errorf("invalid IP address or hostname: %s", s)
					}

					for _, addr := range ipAddrs {
						ipRanges = append(ipRanges, IPRange{Lower: addr, Upper: addr})
					}
				}
			} else {
				// string is CIDR block like 192.168.0.0/16 or fd03:b188:0f3c:9ec4::/64
				rangeLo, network, err := net.ParseCIDR(s)
				if err != nil {
					return nil, errors.Errorf("invalid CIDR block: %s", s)
				}

				addrLen, maskLen := len(rangeLo), len(network.Mask)
				rangeHi := make(net.IP, len(rangeLo))
				copy(rangeHi, rangeLo)
				for i := 1; i <= maskLen; i++ {
					rangeHi[addrLen-i] |= ^network.Mask[maskLen-i]
				}

				ipRanges = append(ipRanges, IPRange{Lower: rangeLo, Upper: rangeHi})
			}
		case len(tuple) == 2:
			// string is range like a.b.c.d:w.x.y.z
			rangeLo := net.ParseIP(tuple[0])
			rangeHi := net.ParseIP(tuple[1])
			switch {
			case rangeLo == nil:
				return nil, errors.Errorf("invalid IP address: %s", tuple[0])
			case rangeHi == nil:
				return nil, errors.Errorf("invalid IP address: %s", tuple[1])
			case bytes.Compare(rangeLo, rangeHi) > 0:
				return nil, errors.Errorf("inverted IP address range: %s", s)
			}
			ipRanges = append(ipRanges, IPRange{Lower: rangeLo, Upper: rangeHi})
		default:
			return nil, errors.Errorf("invalid IP address range: %s", s)
		}
	}

	return ipRanges, nil
}

func SetupListener(addr string, port int, kind string) (listener net.Listener, err error) {
	laddr := fmt.Sprintf("%s:%d", addr, port)
	glog.Infof("Setting up %s listener at: %v\n", kind, laddr)
	return net.Listen("tcp", laddr)
}