/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements. See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership. The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License. You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package thrift

import (
	"context"
	"fmt"
)

type TDebugProtocol struct {
	Delegate  TProtocol
	LogPrefix string
	Logger    Logger
}

type TDebugProtocolFactory struct {
	Underlying TProtocolFactory
	LogPrefix  string
	Logger     Logger
}

// NewTDebugProtocolFactory creates a TDebugProtocolFactory.
//
// Deprecated: Please use NewTDebugProtocolFactoryWithLogger or the struct
// itself instead. This version will use the default logger from standard
// library.
func NewTDebugProtocolFactory(underlying TProtocolFactory, logPrefix string) *TDebugProtocolFactory {
	return &TDebugProtocolFactory{
		Underlying: underlying,
		LogPrefix:  logPrefix,
		Logger:     StdLogger(nil),
	}
}

// NewTDebugProtocolFactoryWithLogger creates a TDebugProtocolFactory.
func NewTDebugProtocolFactoryWithLogger(underlying TProtocolFactory, logPrefix string, logger Logger) *TDebugProtocolFactory {
	return &TDebugProtocolFactory{
		Underlying: underlying,
		LogPrefix:  logPrefix,
		Logger:     logger,
	}
}

func (t *TDebugProtocolFactory) GetProtocol(trans TTransport) TProtocol {
	return &TDebugProtocol{
		Delegate:  t.Underlying.GetProtocol(trans),
		LogPrefix: t.LogPrefix,
		Logger:    fallbackLogger(t.Logger),
	}
}

func (tdp *TDebugProtocol) logf(format string, v ...interface{}) {
	fallbackLogger(tdp.Logger)(fmt.Sprintf(format, v...))
}

func (tdp *TDebugProtocol) WriteMessageBegin(name string, typeId TMessageType, seqid int32) error {
	err := tdp.Delegate.WriteMessageBegin(name, typeId, seqid)
	tdp.logf("%sWriteMessageBegin(name=%#v, typeId=%#v, seqid=%#v) => %#v", tdp.LogPrefix, name, typeId, seqid, err)
	return err
}
func (tdp *TDebugProtocol) WriteMessageEnd() error {
	err := tdp.Delegate.WriteMessageEnd()
	tdp.logf("%sWriteMessageEnd() => %#v", tdp.LogPrefix, err)
	return err
}
func (tdp *TDebugProtocol) WriteStructBegin(name string) error {
	err := tdp.Delegate.WriteStructBegin(name)
	tdp.logf("%sWriteStructBegin(name=%#v) => %#v", tdp.LogPrefix, name, err)
	return err
}
func (tdp *TDebugProtocol) WriteStructEnd() error {
	err := tdp.Delegate.WriteStructEnd()
	tdp.logf("%sWriteStructEnd() => %#v", tdp.LogPrefix, err)
	return err
}
func (tdp *TDebugProtocol) WriteFieldBegin(name string, typeId TType, id int16) error {
	err := tdp.Delegate.WriteFieldBegin(name, typeId, id)
	tdp.logf("%sWriteFieldBegin(name=%#v, typeId=%#v, id%#v) => %#v", tdp.LogPrefix, name, typeId, id, err)
	return err
}
func (tdp *TDebugProtocol) WriteFieldEnd() error {
	err := tdp.Delegate.WriteFieldEnd()
	tdp.logf("%sWriteFieldEnd() => %#v", tdp.LogPrefix, err)
	return err
}
func (tdp *TDebugProtocol) WriteFieldStop() error {
	err := tdp.Delegate.WriteFieldStop()
	tdp.logf("%sWriteFieldStop() => %#v", tdp.LogPrefix, err)
	return err
}
func (tdp *TDebugProtocol) WriteMapBegin(keyType TType, valueType TType, size int) error {
	err := tdp.Delegate.WriteMapBegin(keyType, valueType, size)
	tdp.logf("%sWriteMapBegin(keyType=%#v, valueType=%#v, size=%#v) => %#v", tdp.LogPrefix, keyType, valueType, size, err)
	return err
}
func (tdp *TDebugProtocol) WriteMapEnd() error {
	err := tdp.Delegate.WriteMapEnd()
	tdp.logf("%sWriteMapEnd() => %#v", tdp.LogPrefix, err)
	return err
}
func (tdp *TDebugProtocol) WriteListBegin(elemType TType, size int) error {
	err := tdp.Delegate.WriteListBegin(elemType, size)
	tdp.logf("%sWriteListBegin(elemType=%#v, size=%#v) => %#v", tdp.LogPrefix, elemType, size, err)
	return err
}
func (tdp *TDebugProtocol) WriteListEnd() error {
	err := tdp.Delegate.WriteListEnd()
	tdp.logf("%sWriteListEnd() => %#v", tdp.LogPrefix, err)
	return err
}
func (tdp *TDebugProtocol) WriteSetBegin(elemType TType, size int) error {
	err := tdp.Delegate.WriteSetBegin(elemType, size)
	tdp.logf("%sWriteSetBegin(elemType=%#v, size=%#v) => %#v", tdp.LogPrefix, elemType, size, err)
	return err
}
func (tdp *TDebugProtocol) WriteSetEnd() error {
	err := tdp.Delegate.WriteSetEnd()
	tdp.logf("%sWriteSetEnd() => %#v", tdp.LogPrefix, err)
	return err
}
func (tdp *TDebugProtocol) WriteBool(value bool) error {
	err := tdp.Delegate.WriteBool(value)
	tdp.logf("%sWriteBool(value=%#v) => %#v", tdp.LogPrefix, value, err)
	return err
}
func (tdp *TDebugProtocol) WriteByte(value int8) error {
	err := tdp.Delegate.WriteByte(value)
	tdp.logf("%sWriteByte(value=%#v) => %#v", tdp.LogPrefix, value, err)
	return err
}
func (tdp *TDebugProtocol) WriteI16(value int16) error {
	err := tdp.Delegate.WriteI16(value)
	tdp.logf("%sWriteI16(value=%#v) => %#v", tdp.LogPrefix, value, err)
	return err
}
func (tdp *TDebugProtocol) WriteI32(value int32) error {
	err := tdp.Delegate.WriteI32(value)
	tdp.logf("%sWriteI32(value=%#v) => %#v", tdp.LogPrefix, value, err)
	return err
}
func (tdp *TDebugProtocol) WriteI64(value int64) error {
	err := tdp.Delegate.WriteI64(value)
	tdp.logf("%sWriteI64(value=%#v) => %#v", tdp.LogPrefix, value, err)
	return err
}
func (tdp *TDebugProtocol) WriteDouble(value float64) error {
	err := tdp.Delegate.WriteDouble(value)
	tdp.logf("%sWriteDouble(value=%#v) => %#v", tdp.LogPrefix, value, err)
	return err
}
func (tdp *TDebugProtocol) WriteString(value string) error {
	err := tdp.Delegate.WriteString(value)
	tdp.logf("%sWriteString(value=%#v) => %#v", tdp.LogPrefix, value, err)
	return err
}
func (tdp *TDebugProtocol) WriteBinary(value []byte) error {
	err := tdp.Delegate.WriteBinary(value)
	tdp.logf("%sWriteBinary(value=%#v) => %#v", tdp.LogPrefix, value, err)
	return err
}

func (tdp *TDebugProtocol) ReadMessageBegin() (name string, typeId TMessageType, seqid int32, err error) {
	name, typeId, seqid, err = tdp.Delegate.ReadMessageBegin()
	tdp.logf("%sReadMessageBegin() (name=%#v, typeId=%#v, seqid=%#v, err=%#v)", tdp.LogPrefix, name, typeId, seqid, err)
	return
}
func (tdp *TDebugProtocol) ReadMessageEnd() (err error) {
	err = tdp.Delegate.ReadMessageEnd()
	tdp.logf("%sReadMessageEnd() err=%#v", tdp.LogPrefix, err)
	return
}
func (tdp *TDebugProtocol) ReadStructBegin() (name string, err error) {
	name, err = tdp.Delegate.ReadStructBegin()
	tdp.logf("%sReadStructBegin() (name%#v, err=%#v)", tdp.LogPrefix, name, err)
	return
}
func (tdp *TDebugProtocol) ReadStructEnd() (err error) {
	err = tdp.Delegate.ReadStructEnd()
	tdp.logf("%sReadStructEnd() err=%#v", tdp.LogPrefix, err)
	return
}
func (tdp *TDebugProtocol) ReadFieldBegin() (name string, typeId TType, id int16, err error) {
	name, typeId, id, err = tdp.Delegate.ReadFieldBegin()
	tdp.logf("%sReadFieldBegin() (name=%#v, typeId=%#v, id=%#v, err=%#v)", tdp.LogPrefix, name, typeId, id, err)
	return
}
func (tdp *TDebugProtocol) ReadFieldEnd() (err error) {
	err = tdp.Delegate.ReadFieldEnd()
	tdp.logf("%sReadFieldEnd() err=%#v", tdp.LogPrefix, err)
	return
}
func (tdp *TDebugProtocol) ReadMapBegin() (keyType TType, valueType TType, size int, err error) {
	keyType, valueType, size, err = tdp.Delegate.ReadMapBegin()
	tdp.logf("%sReadMapBegin() (keyType=%#v, valueType=%#v, size=%#v, err=%#v)", tdp.LogPrefix, keyType, valueType, size, err)
	return
}
func (tdp *TDebugProtocol) ReadMapEnd() (err error) {
	err = tdp.Delegate.ReadMapEnd()
	tdp.logf("%sReadMapEnd() err=%#v", tdp.LogPrefix, err)
	return
}
func (tdp *TDebugProtocol) ReadListBegin() (elemType TType, size int, err error) {
	elemType, size, err = tdp.Delegate.ReadListBegin()
	tdp.logf("%sReadListBegin() (elemType=%#v, size=%#v, err=%#v)", tdp.LogPrefix, elemType, size, err)
	return
}
func (tdp *TDebugProtocol) ReadListEnd() (err error) {
	err = tdp.Delegate.ReadListEnd()
	tdp.logf("%sReadListEnd() err=%#v", tdp.LogPrefix, err)
	return
}
func (tdp *TDebugProtocol) ReadSetBegin() (elemType TType, size int, err error) {
	elemType, size, err = tdp.Delegate.ReadSetBegin()
	tdp.logf("%sReadSetBegin() (elemType=%#v, size=%#v, err=%#v)", tdp.LogPrefix, elemType, size, err)
	return
}
func (tdp *TDebugProtocol) ReadSetEnd() (err error) {
	err = tdp.Delegate.ReadSetEnd()
	tdp.logf("%sReadSetEnd() err=%#v", tdp.LogPrefix, err)
	return
}
func (tdp *TDebugProtocol) ReadBool() (value bool, err error) {
	value, err = tdp.Delegate.ReadBool()
	tdp.logf("%sReadBool() (value=%#v, err=%#v)", tdp.LogPrefix, value, err)
	return
}
func (tdp *TDebugProtocol) ReadByte() (value int8, err error) {
	value, err = tdp.Delegate.ReadByte()
	tdp.logf("%sReadByte() (value=%#v, err=%#v)", tdp.LogPrefix, value, err)
	return
}
func (tdp *TDebugProtocol) ReadI16() (value int16, err error) {
	value, err = tdp.Delegate.ReadI16()
	tdp.logf("%sReadI16() (value=%#v, err=%#v)", tdp.LogPrefix, value, err)
	return
}
func (tdp *TDebugProtocol) ReadI32() (value int32, err error) {
	value, err = tdp.Delegate.ReadI32()
	tdp.logf("%sReadI32() (value=%#v, err=%#v)", tdp.LogPrefix, value, err)
	return
}
func (tdp *TDebugProtocol) ReadI64() (value int64, err error) {
	value, err = tdp.Delegate.ReadI64()
	tdp.logf("%sReadI64() (value=%#v, err=%#v)", tdp.LogPrefix, value, err)
	return
}
func (tdp *TDebugProtocol) ReadDouble() (value float64, err error) {
	value, err = tdp.Delegate.ReadDouble()
	tdp.logf("%sReadDouble() (value=%#v, err=%#v)", tdp.LogPrefix, value, err)
	return
}
func (tdp *TDebugProtocol) ReadString() (value string, err error) {
	value, err = tdp.Delegate.ReadString()
	tdp.logf("%sReadString() (value=%#v, err=%#v)", tdp.LogPrefix, value, err)
	return
}
func (tdp *TDebugProtocol) ReadBinary() (value []byte, err error) {
	value, err = tdp.Delegate.ReadBinary()
	tdp.logf("%sReadBinary() (value=%#v, err=%#v)", tdp.LogPrefix, value, err)
	return
}
func (tdp *TDebugProtocol) Skip(fieldType TType) (err error) {
	err = tdp.Delegate.Skip(fieldType)
	tdp.logf("%sSkip(fieldType=%#v) (err=%#v)", tdp.LogPrefix, fieldType, err)
	return
}
func (tdp *TDebugProtocol) Flush(ctx context.Context) (err error) {
	err = tdp.Delegate.Flush(ctx)
	tdp.logf("%sFlush() (err=%#v)", tdp.LogPrefix, err)
	return
}

func (tdp *TDebugProtocol) Transport() TTransport {
	return tdp.Delegate.Transport()
}
