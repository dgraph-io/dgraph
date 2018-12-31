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

// // write uses the data length as delimiter.
// // XXX: we could use CRC for restore.
// func (w *writer) write(kv *bpb.KV) error {
// 	if err := binary.Write(w.h, binary.LittleEndian, uint64(kv.Size())); err != nil {
// 		return err
// 	}
// 	b, err := kv.Marshal()
// 	if err != nil {
// 		return err
// 	}
// 	_, err = w.h.Write(b)
// 	return err
// }

// // Send implements the stream.kvStream interface.
// // It writes the received KV to the target as a delimited binary chain.
// // Returns error if the writing fails, nil on success.
// func (w *writer) Send(kvs *pb.KVS) error {
// 	var err error
// 	for _, kv := range kvs.Kv {
// 		err = w.write(kv)
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }
