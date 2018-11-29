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
	"io"
	"net/url"
	"strings"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
)

// handler interface is implemented by URI scheme handlers.
type handler interface {
	// Handlers know how to Write and Close to their target.
	io.WriteCloser
	// Session receives the host and path of the target. It should get all its configuration
	// from the environment.
	Open(*url.URL, *Request) error
}

// writer handles the writes from stream.Orchestrate. It implements the kvStream interface.
type writer struct {
	h handler
}

// newWriter parses the requested target URI, finds a handler and then tries to create a session.
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
func (r *Request) newWriter() (*writer, error) {
	uri, err := url.Parse(r.Backup.Target)
	if err != nil {
		return nil, err
	}

	// find handler for this URI scheme
	var h handler
	switch uri.Scheme {
	case "file":
		h = &fileHandler{}
	case "s3":
		h = &s3Handler{}
	case "http", "https":
		if strings.HasPrefix(uri.Host, "s3") &&
			strings.HasSuffix(uri.Host, ".amazonaws.com") {
			h = &s3Handler{}
		}
	}
	if h == nil {
		return nil, x.Errorf("Unable to handle url: %v", uri)
	}

	if err := h.Open(uri, r); err != nil {
		return nil, err
	}

	return &writer{h: h}, nil
}

func (w *writer) flush() error {
	glog.V(2).Infof("Backup closing handler.")
	return w.h.Close()
}

// write uses the data length as delimiter.
// XXX: we could use CRC for restore.
func (w *writer) write(kv *pb.KV) error {
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
// It writes the received KV to the target as a delimited binary chain.
// Returns error if the writing fails, nil on success.
func (w *writer) Send(kvs *pb.KVS) error {
	var err error
	for _, kv := range kvs.Kv {
		err = w.write(kv)
		if err != nil {
			return err
		}
	}
	return nil
}
