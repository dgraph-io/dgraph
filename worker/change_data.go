// +build oss

/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

package worker

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/x"
)

var logger *x.Logger
var lis chan msg
var CdcIndex uint64 = 1

func init() {
	var err error
	logger, err = x.InitLogger("cdc", "cdc.log", nil, false)
	x.Check(err)
	lis = make(chan msg, 2000)
	go listenAndDo()
}

type msg struct {
	m   string
	idx uint64
}

func WriteCDC(m string, idx uint64) {
	fmt.Println("writing CDC")
	time.Sleep(time.Second * 3)
	logger.AuditI(m)
	logger.Sync()
	atomic.StoreUint64(&CdcIndex, idx)
	//lis <- msg{
	//	m:   m,
	//	idx: idx,
	//}
}

func listenAndDo() {
	for {
		select {
		case m := <-lis:
			time.Sleep(time.Second * 10)
			logger.AuditI(m.m)
			logger.Sync()
			atomic.StoreUint64(&CdcIndex, m.idx)
		}
	}
}
