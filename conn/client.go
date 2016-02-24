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
	n, err := c.Rwc.Read(buf)
	if n != int(c.payloadLen) {
		return fmt.Errorf("ClientCodec expected: %d. Got: %d\n", c.payloadLen, n)
	}

	reply := body.(*Reply)
	reply.Data = buf
	return err
}

func (c *ClientCodec) Close() error {
	return c.Rwc.Close()
}
