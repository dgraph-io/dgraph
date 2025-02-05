/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"log"

	"github.com/golang/glog"
)

// ToGlog is a logger that forwards the output to glog.
type ToGlog struct{}

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
