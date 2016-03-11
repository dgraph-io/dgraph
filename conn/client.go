/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package conn

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/rpc"
)

type ClientCodec struct {
	Rwc        io.ReadWriteCloser
	payloadLen int32
}

func (c *ClientCodec) WriteRequest(r *rpc.Request, body interface{}) error {
	if body == nil {
		return fmt.Errorf("Nil request body from client.")
	}

	query := body.(*Query)
	if err := writeHeader(c.Rwc, r.Seq, r.ServiceMethod, query.Data); err != nil {
		return err
	}
	n, err := c.Rwc.Write(query.Data)
	if n != len(query.Data) {
		return errors.New("Unable to write payload.")
	}
	return err
}

func (c *ClientCodec) ReadResponseHeader(r *rpc.Response) error {
	if len(r.Error) > 0 {
		log.Fatal("client got response error: " + r.Error)
	}
	if err := parseHeader(c.Rwc, &r.Seq,
		&r.ServiceMethod, &c.payloadLen); err != nil {
		return err
	}
	return nil
}

func (c *ClientCodec) ReadResponseBody(body interface{}) error {
	buf := make([]byte, c.payloadLen)
	_, err := io.ReadFull(c.Rwc, buf)
	reply := body.(*Reply)
	reply.Data = buf
	return err
}

func (c *ClientCodec) Close() error {
	return c.Rwc.Close()
}
