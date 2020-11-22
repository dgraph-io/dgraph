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

package x

import (
	"log"

	"github.com/golang/glog"
)

// ToGlog is a logger that forwards the output to glog.
type ToGlog struct {
}

func (rl *ToGlog) Debug(v ...interface{})                   { glog.V(3).Info(v...) }
func (rl *ToGlog) Debugf(format string, v ...interface{})   { glog.V(3).Infof(format, v...) }
func (rl *ToGlog) Error(v ...interface{})                   { glog.Error(v...) }
func (rl *ToGlog) Errorf(format string, v ...interface{})   { glog.Errorf(format, v...) }
func (rl *ToGlog) Info(v ...interface{})                    { glog.Info(v...) }
func (rl *ToGlog) Infof(format string, v ...interface{})    { glog.Infof(format, v...) }
func (rl *ToGlog) Warning(v ...interface{})                 { glog.Warning(v...) }
func (rl *ToGlog) Warningf(format string, v ...interface{}) { glog.Warningf(format, v...) }
func (rl *ToGlog) Fatal(v ...interface{})                   { glog.Fatal(v...) }
func (rl *ToGlog) Fatalf(format string, v ...interface{})   { glog.Fatalf(format, v...) }
func (rl *ToGlog) Panic(v ...interface{})                   { log.Panic(v...) }
func (rl *ToGlog) Panicf(format string, v ...interface{})   { log.Panicf(format, v...) }
