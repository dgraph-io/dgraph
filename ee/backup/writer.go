// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package backup

import (
	"encoding/binary"
	"net/url"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
)

// KVWriter implements the stream.kvStream interface.
type KVWriter struct {
	h handler
}

// newWriter parses the requested source URI, finds a handler and then tries to create a session
// for writing.
//
// Target URI formats:
//   [scheme]://[host]/[path]?[args]
//   [scheme]:///[path]?[args]
//   /[path]?[args] (only for local or NFS)
//
// Target URI parts:
//   scheme - service handler, one of: "s3", "gs", "az", "http", "file"
//     host - remote address. ex: "dgraph.s3.amazonaws.com"
//     path - directory, bucket or container at target. ex: "/dgraph/backups/"
//     args - specific arguments that are ok to appear in logs.
//
// Global args (might not be support by all handlers):
//     secure - true|false turn on/off TLS.
//   compress - true|false turn on/off data compression.
//
// Examples:
//   s3://dgraph.s3.amazonaws.com/dgraph/backups?secure=true
//   gs://dgraph/backups/
//   as://dgraph-container/backups/
//   http://backups.dgraph.io/upload
//   file:///tmp/dgraph/backups or /tmp/dgraph/backups?compress=gzip
func (r *Request) newWriter() (*KVWriter, error) {
	uri, err := url.Parse(r.Backup.Location)
	if err != nil {
		return nil, err
	}

	h := getHandler(uri.Scheme)
	if h == nil {
		return nil, x.Errorf("Unsupported URI: %v", uri)
	}

	if err := h.Create(uri, r); err != nil {
		return nil, err
	}

	return &KVWriter{h: h}, nil
}

func (w *KVWriter) Close() error {
	glog.V(2).Infof("Backup closing handler.")
	return w.h.Close()
}

// Write implements the io.Writer interface backed by an URI handler.
func (w *KVWriter) Write(b []byte) (int, error) {
	return w.h.Write(b)
}

// writeKV writes a single KV via handler using the data length as delimiter.
func (w *KVWriter) writeKV(kv *pb.KV) error {
	if err := binary.Write(w.h, binary.LittleEndian, uint64(kv.Size())); err != nil {
		return err
	}
	b, err := kv.Marshal()
	if err != nil {
		return err
	}
	_, err = w.h.Write(b)
	return err
}

// Send implements the stream.kvStream interface.
// It writes the received KVS to the target as a delimited binary chain.
// Returns error if the writing fails, nil on success.
func (w *KVWriter) Send(kvs *pb.KVS) error {
	var err error
	for _, kv := range kvs.Kv {
		err = w.writeKV(kv)
		if err != nil {
			return err
		}
	}
	return nil
}
