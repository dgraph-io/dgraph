/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package resolve

import (
	"sync"

	"go.opencensus.io/trace"
)

type Exporter struct {
	sync.RWMutex

	// map of span id to SpanData.
	spans map[[8]byte]*trace.SpanData
}

func NewExporter() *Exporter {
	return &Exporter{
		spans: make(map[[8]byte]*trace.SpanData),
	}
}

func (e *Exporter) ExportSpan(data *trace.SpanData) {
	sid := data.SpanContext.SpanID
	if len(sid) == 0 {
		return
	}

	e.Lock()
	defer e.Unlock()
	e.spans[sid] = data
}

func (e *Exporter) Span(id [8]byte) *trace.SpanData {
	e.RLock()
	defer e.RUnlock()
	return e.spans[id]
}
