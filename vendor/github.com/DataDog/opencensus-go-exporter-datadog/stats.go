// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package datadog

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-go/statsd"
	"go.opencensus.io/stats/view"
)

const (
	// DefaultStatsAddrUDP specifies the default protocol (UDP) and address
	// for the DogStatsD service.
	DefaultStatsAddrUDP = "localhost:8125"

	// DefaultStatsAddrUDS specifies the default socket address for the
	// DogStatsD service over UDS. Only useful for platforms supporting unix
	// sockets.
	DefaultStatsAddrUDS = "unix:///var/run/datadog/dsd.socket"
)

// collector implements statsd.Client
type statsExporter struct {
	opts     Options
	client   *statsd.Client
	mu       sync.Mutex // mu guards viewData
	viewData map[string]*view.Data
}

func newStatsExporter(o Options) (*statsExporter, error) {
	endpoint := o.StatsAddr
	if endpoint == "" {
		endpoint = DefaultStatsAddrUDP
	}

	client, err := statsd.New(endpoint)
	if err != nil {
		return nil, err
	}

	return &statsExporter{
		opts:     o,
		viewData: make(map[string]*view.Data),
		client:   client,
	}, nil
}

func (s *statsExporter) addViewData(vd *view.Data) {
	sig := viewSignature(s.opts.Namespace, vd.View)
	s.mu.Lock()
	s.viewData[sig] = vd
	s.mu.Unlock()

	var lastErr error
	for _, row := range vd.Rows {
		if err := s.submitMetric(vd.View, row, sig); err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		s.opts.onError(lastErr) // Only report last error.
	}
}

func (s *statsExporter) submitMetric(v *view.View, row *view.Row, metricName string) error {
	var err error
	const rate = float64(1)
	client := s.client
	opt := s.opts
	tags := []string{}

	switch data := row.Data.(type) {
	case *view.CountData:
		return client.Gauge(metricName, float64(data.Value), opt.tagMetrics(row.Tags, tags), rate)

	case *view.SumData:
		return client.Gauge(metricName, float64(data.Value), opt.tagMetrics(row.Tags, tags), rate)

	case *view.LastValueData:
		return client.Gauge(metricName, float64(data.Value), opt.tagMetrics(row.Tags, tags), rate)

	case *view.DistributionData:
		var metrics = map[string]float64{
			"min":             data.Min,
			"max":             data.Max,
			"count":           float64(data.Count),
			"avg":             data.Mean,
			"squared_dev_sum": data.SumOfSquaredDev,
		}

		for name, value := range metrics {
			err = client.Gauge(metricName+"."+name, value, opt.tagMetrics(row.Tags, tags), rate)
		}

		for x := range data.CountPerBucket {
			addlTags := []string{"bucket_idx:" + fmt.Sprint(x)}
			err = client.Gauge(metricName+".count_per_bucket", float64(data.CountPerBucket[x]), opt.tagMetrics(row.Tags, addlTags), rate)
		}
		return err
	default:
		return fmt.Errorf("aggregation %T is not supported", v.Aggregation)
	}
}
