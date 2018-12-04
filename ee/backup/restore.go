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
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

const bufSize = 16 << 10

func (r *Request) Restore(ctx context.Context) error {
	f, err := r.openLocation(r.Backup.Source)
	if err != nil {
		return err
	}

	br := bufio.NewReaderSize(f.h, bufSize)
	errChan := make(chan error, 1)

	var (
		bb      bytes.Buffer
		sz      uint64
		entries []*pb.KV
	)
	for {
		err = binary.Read(br, binary.LittleEndian, &sz)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		e := &pb.KV{}
		n, err := bb.ReadFrom(io.LimitReader(br, int64(sz)))
		if err != nil {
			return err
		}
		if n != int64(sz) {
			return x.Errorf("Restore failed read. Expected %d bytes but got %d instead.", sz, n)
		}
		if err = e.Unmarshal((&bb).Bytes()); err != nil {
			return err
		}
		entries = append(entries, e)
	}

	select {
	case err := <-errChan:
		return err
	default:
		// Mark all versions done up until nextTxnTs.
	}

	return nil
}
