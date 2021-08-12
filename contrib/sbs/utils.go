package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync/atomic"
)

type histogram struct {
	bins  []float64
	count []uint64
	total uint64
}

func newHistogram(bins []float64) *histogram {
	return &histogram{
		bins:  bins,
		count: make([]uint64, len(bins)),
	}
}

func (h *histogram) add(v float64) {
	idx := 0
	for i := 0; i < len(h.bins); i++ {
		if i+1 >= len(h.bins) {
			idx = i
			break
		}
		if h.bins[i] <= v && h.bins[i+1] > v {
			idx = i
			break
		}
	}
	atomic.AddUint64(&h.count[idx], 1)
	atomic.AddUint64(&h.total, 1)
}

func (h *histogram) show() {
	fmt.Printf("-------------- Histogram --------------\nTotal samples: %d\n",
		atomic.LoadUint64(&h.total))
	for i := 0; i < len(h.bins); i++ {
		pert := float64(atomic.LoadUint64(&h.count[i])) / float64(atomic.LoadUint64(&h.total))
		if i+1 >= len(h.bins) {
			fmt.Printf("%.2f - infi  --> %.2f\n", h.bins[i], pert)
			continue
		}
		fmt.Printf("%.2f - %.2f  --> %.2f\n", h.bins[i], h.bins[i+1], pert)
	}
}

func areEqualJSON(s1, s2 string) bool {
	var o1, o2 interface{}

	err := json.Unmarshal([]byte(s1), &o1)
	if err != nil {
		return false
	}
	err = json.Unmarshal([]byte(s2), &o2)
	if err != nil {
		return false
	}
	return reflect.DeepEqual(o1, o2)
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
