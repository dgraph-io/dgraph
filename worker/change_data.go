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
	"math"
)

type ChangeData struct {
}

func initChangeDataCapture() *ChangeData {
	return nil
}

func (cd *ChangeData) getCDCMinReadTs() uint64 {
	return math.MaxUint64
}

func (cd *ChangeData) updateMinReadTs(ts uint64) {
	return
}

func (cd *ChangeData) Close() {
	return
}

// todo: test cases old cluster restart, live loader, bulk loader, backup restore etc
func (cd *ChangeData) processCDCEvents() {
	return
}
