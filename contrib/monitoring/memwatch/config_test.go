/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"reflect"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestParseConfigDefaults(t *testing.T) {
	c := defaultConfig()
	if c.Mode != "auto" || c.AlphaURL != "http://localhost:8080" || c.Interval != time.Second {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if !reflect.DeepEqual(c.Thresholds, []float64{0.70, 0.85, 0.95}) {
		t.Fatalf("thresholds = %v", c.Thresholds)
	}
	if c.RiseBytes != 256<<20 || c.MaxBytes != 1<<30 || c.LimitBytes != 0 {
		t.Fatalf("rise=%d max=%d limit=%d", c.RiseBytes, c.MaxBytes, c.LimitBytes)
	}
	if !reflect.DeepEqual(c.ProcMatch, []string{"dgraph", "alpha"}) {
		t.Fatalf("proc match = %v", c.ProcMatch)
	}
}

func TestParseConfigEnvAndFlags(t *testing.T) {
	c, err := parseConfig(
		[]string{"--cooldown=30s", "--alpha-url=http://alpha:8080/"},
		env(map[string]string{
			"MEMWATCH_LIMIT_BYTES": "2147483648",
			"MEMWATCH_COOLDOWN":    "5m",
			"MEMWATCH_THRESHOLDS":  "0.9, 0.5",
		}))
	if err != nil {
		t.Fatal(err)
	}
	if c.LimitBytes != 2<<30 {
		t.Errorf("limit from env = %d", c.LimitBytes)
	}
	if c.Cooldown != 30*time.Second {
		t.Errorf("flag should override env, cooldown = %v", c.Cooldown)
	}
	if !reflect.DeepEqual(c.Thresholds, []float64{0.5, 0.9}) {
		t.Errorf("thresholds should be sorted, got %v", c.Thresholds)
	}
	if c.RiseBytes != c.LimitBytes/5 {
		t.Errorf("auto rise should be 20%% of the limit, got %d", c.RiseBytes)
	}
	if c.AlphaURL != "http://alpha:8080" {
		t.Errorf("trailing slash not trimmed: %q", c.AlphaURL)
	}
}

func TestParseConfigErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--mode=bogus"},
		{"--thresholds=1.5"},
		{"--limit-bytes=lots"},
		{"--ring-size=1"},
		{"--interval=0s"},
		{"--rise-bytes=fast"},
	} {
		if _, err := parseConfig(args, env(nil)); err == nil {
			t.Errorf("%v: expected an error", args)
		}
	}
	if _, err := parseConfig(nil, env(map[string]string{"MEMWATCH_INTERVAL": "soon"})); err == nil {
		t.Error("bad env value: expected an error")
	}
}

func TestParseBytes(t *testing.T) {
	for in, want := range map[string]uint64{
		"0": 0, "1024": 1024, "2Gi": 2 << 30, "512Mi": 512 << 20, "1.5Gi": 3 << 29, "64KiB": 64 << 10,
		"1G": 1e9, "100M": 100e6, "7B": 7, " 3 Mi ": 3 << 20,
	} {
		got, err := parseBytes(in)
		if err != nil || got != want {
			t.Errorf("parseBytes(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "Gi", "-1", "ten"} {
		if _, err := parseBytes(in); err == nil {
			t.Errorf("parseBytes(%q): expected an error", in)
		}
	}
}
