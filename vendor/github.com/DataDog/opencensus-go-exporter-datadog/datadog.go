// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package datadog

import (
	"log"
	"regexp"
	"strings"

	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
)

var (
	_ view.Exporter  = (*Exporter)(nil)
	_ trace.Exporter = (*Exporter)(nil)
)

// Exporter exports stats to Datadog.
type Exporter struct {
	*statsExporter
	*traceExporter
}

// ExportView implements view.Exporter.
func (e *Exporter) ExportView(vd *view.Data) {
	if len(vd.Rows) == 0 {
		return
	}
	e.statsExporter.addViewData(vd)
}

// ExportSpan implements trace.Exporter.
func (e *Exporter) ExportSpan(s *trace.SpanData) {
	e.traceExporter.exportSpan(s)
}

// Stop cleanly stops the exporter, flushing any remaining spans to the transport and
// reporting any errors. Make sure to always call Stop at the end of your program in
// order to not lose any tracing data. Only call Stop once per exporter. Repeated calls
// will cause panic.
func (e *Exporter) Stop() {
	e.traceExporter.stop()
}

// Options contains options for configuring the exporter.
type Options struct {
	// Namespace specifies the namespaces to which metric keys are appended.
	Namespace string

	// Service specifies the service name used for tracing.
	Service string

	// TraceAddr specifies the host[:port] address of the Datadog Trace Agent.
	// It defaults to localhost:8126.
	TraceAddr string

	// StatsAddr specifies the host[:port] address for DogStatsD. It defaults
	// to localhost:8125.
	StatsAddr string

	// OnError specifies a function that will be called if an error occurs during
	// processing stats or metrics.
	OnError func(err error)

	// Tags specifies a set of global tags to attach to each metric.
	Tags []string

	// GlobalTags holds a set of tags that will automatically be applied to all
	// exported spans.
	GlobalTags map[string]interface{}
}

func (o *Options) onError(err error) {
	if o.OnError != nil {
		o.OnError(err)
	} else {
		log.Printf("Failed to export to Datadog: %v\n", err)
	}
}

// NewExporter returns an exporter that exports stats and traces to Datadog.
// When using trace, it is important to call Stop at the end of your program
// for a clean exit and to flush any remaining tracing data to the Datadog agent.
// If an error occurs initializing the stats exporter, the error will be returned
// and the exporter will be nil.
func NewExporter(o Options) (exporter *Exporter, err error) {
	statsExporter, err := newStatsExporter(o)
	if err != nil {
		return nil, err
	}
	return &Exporter{
		statsExporter: statsExporter,
		traceExporter: newTraceExporter(o),
	}, nil
}

// regex pattern
var reg = regexp.MustCompile("[^a-zA-Z0-9]+")

// sanitizeString replaces all non-alphanumerical characters to underscore
func sanitizeString(str string) string {
	return reg.ReplaceAllString(str, "_")
}

// sanitizeMetricName formats the custom namespace and view name to
// Datadog's metric naming convention
func sanitizeMetricName(namespace string, v *view.View) string {
	if namespace != "" {
		namespace = strings.Replace(namespace, " ", "", -1)
		return sanitizeString(namespace) + "." + sanitizeString(v.Name)
	}
	return sanitizeString(v.Name)
}

// viewSignature creates the view signature with custom namespace
func viewSignature(namespace string, v *view.View) string {
	var buf strings.Builder
	buf.WriteString(sanitizeMetricName(namespace, v))
	for _, k := range v.TagKeys {
		buf.WriteString("_" + k.Name())
	}
	return buf.String()
}

// tagMetrics concatenates user input custom tags with row tags
func (o *Options) tagMetrics(rowTags []tag.Tag, addlTags []string) []string {
	finalTags := make([]string, len(o.Tags), len(o.Tags)+len(rowTags)+len(addlTags))
	copy(finalTags, o.Tags)
	for key := range rowTags {
		finalTags = append(finalTags,
			rowTags[key].Key.Name()+":"+rowTags[key].Value)
	}
	finalTags = append(finalTags, addlTags...)
	return finalTags
}
