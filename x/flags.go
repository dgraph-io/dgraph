/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
)

// FillCommonFlags stores flags common to Alpha and Zero.
func FillCommonFlags(flag *pflag.FlagSet) {
	flag.String("my", "",
		"addr:port of this server, so other Dgraph servers can talk to this.")

	// OpenCensus flags.
	flag.Float64("trace", 0.01, "The ratio of queries to trace.")
	flag.String("jaeger.collector", "", "Send opencensus traces to Jaeger.")
	// See https://github.com/DataDog/opencensus-go-exporter-datadog/issues/34
	// about the status of supporting annotation logs through the datadog exporter
	flag.String("datadog.collector", "", "Send opencensus traces to Datadog. As of now, the trace"+
		" exporter does not support annotation logs and would discard them.")

	// Performance flags.
	flag.String("survive", "process",
		`Choose between "process" or "filesystem".
		If set to "process", there would be no data loss in case of process crash, but the
		behavior would be indeterministic in case of filesystem crash. If set to "filesystem",
		blocking sync would be called after every write, hence guaranteeing no data loss in case
		of hard reboot. Most users should be OK with choosing "process".
		`)

	// Cache flags.
	flag.Int64("cache_mb", 1024, "Total size of cache (in MB) to be used in Dgraph.")

	// Telemetry.
	flag.Bool("telemetry", true, "Send anonymous telemetry data to Dgraph devs.")
	flag.Bool("enable_sentry", true, "Turn on/off sending crash events to Sentry.")
}

func parseFlag(flag string) map[string]string {
	kvm := make(map[string]string)
	for _, kv := range strings.Split(flag, ";") {
		if strings.TrimSpace(kv) == "" {
			continue
		}
		splits := strings.SplitN(kv, "=", 2)
		k := strings.TrimSpace(splits[0])
		k = strings.ToLower(k)
		k = strings.ReplaceAll(k, "_", "-")
		kvm[k] = strings.TrimSpace(splits[1])
	}
	return kvm
}

type SuperFlag struct {
	m map[string]string
}

func NewSuperFlag(flag string) *SuperFlag {
	return &SuperFlag{
		m: parseFlag(flag),
	}
}
func (sf *SuperFlag) String() string {
	if sf == nil {
		return ""
	}
	var kvs []string
	for k, v := range sf.m {
		kvs = append(kvs, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(kvs, "; ")
}
func (sf *SuperFlag) MergeAndCheckDefault(flag string) *SuperFlag {
	if sf == nil {
		sf = &SuperFlag{
			m: parseFlag(flag),
		}
		return sf
	}
	numKeys := len(sf.m)
	src := parseFlag(flag)
	for k := range src {
		if _, ok := sf.m[k]; ok {
			numKeys--
		}
	}
	if numKeys != 0 {
		msg := fmt.Sprintf("Found invalid options in %s. Valid options: %v", sf, flag)
		panic(msg)
	}
	for k, v := range src {
		if _, ok := sf.m[k]; !ok {
			sf.m[k] = v
		}
	}
	return sf
}
func (sf *SuperFlag) Get(opt string) string {
	if sf == nil {
		return ""
	}
	return sf.m[opt]
}
func (sf *SuperFlag) GetBool(opt string) bool {
	val := sf.Get(opt)
	if val == "" {
		return false
	}
	b, err := strconv.ParseBool(val)
	Checkf(err, "Unable to parse %s as bool for key: %s. Options: %s\n", val, opt, sf)
	return b
}
func (sf *SuperFlag) GetUint64(opt string) uint64 {
	val := sf.Get(opt)
	if val == "" {
		return 0
	}
	u, err := strconv.ParseUint(val, 0, 64)
	Checkf(err, "Unable to parse %s as uint64 for key: %s. Options: %s\n", val, opt, sf)
	return u
}
func (sf *SuperFlag) GetUint32(opt string) uint32 {
	val := sf.Get(opt)
	if val == "" {
		return 0
	}
	u, err := strconv.ParseUint(val, 0, 32)
	Checkf(err, "Unable to parse %s as uint32 for key: %s. Options: %s\n", val, opt, sf)
	return uint32(u)
}
