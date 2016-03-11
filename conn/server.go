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
	"io"
	"log"
	"net/rpc"
)

type ServerCodec struct {
	Rwc        io.ReadWriteCloser
	payloadLen int32
}

func (c *ServerCodec) ReadRequestHeader(r *rpc.Request) error {
	return parseHeader(c.Rwc, &r.Seq, &r.ServiceMethod, &c.payloadLen)
}

func (c *ServerCodec) ReadRequestBody(data interface{}) error {
	b := make([]byte, c.payloadLen)
	_, err := io.ReadFull(c.Rwc, b)
	if err != nil {
		return err
	}

	if data == nil {
		// If data is nil, discard this request.
		return nil
	}
	query := data.(*Query)
	query.Data = b
	return nil
}

func (c *ServerCodec) WriteResponse(resp *rpc.Response,
	data interface{}) error {

	if len(resp.Error) > 0 {
		log.Fatal("Response has error: " + resp.Error)
	}
	if data == nil {
		log.Fatal("Worker write response data is nil")
	}
	reply, ok := data.(*Reply)
	if !ok {
		log.Fatal("Unable to convert to reply")
	}

	if err := writeHeader(c.Rwc, resp.Seq,
		resp.ServiceMethod, reply.Data); err != nil {
		return err
	}

	_, err := c.Rwc.Write(reply.Data)
	return err
}

func (c *ServerCodec) Close() error {
	return c.Rwc.Close()
}
