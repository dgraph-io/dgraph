package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net/rpc"
)

type ccodec struct {
	rwc        io.ReadWriteCloser
	ebuf       *bufio.Writer
	payloadLen int32
}

func writeHeader(rwc io.ReadWriteCloser, seq uint64,
	method string, data []byte) error {

	var bh bytes.Buffer
	var rerr error

	setError(&rerr, binary.Write(&bh, binary.LittleEndian, seq))
	setError(&rerr, binary.Write(&bh, binary.LittleEndian, int32(len(method))))
	setError(&rerr, binary.Write(&bh, binary.LittleEndian, int32(len(data))))
	_, err := bh.Write([]byte(method))
	setError(&rerr, err)
	if rerr != nil {
		return rerr
	}
	_, err = rwc.Write(bh.Bytes())
	return err
}

func parseHeader(rwc io.ReadWriteCloser, seq *uint64, method *string, plen *int32) error {
	var err error
	var sz int32
	setError(&err, binary.Read(rwc, binary.LittleEndian, seq))
	setError(&err, binary.Read(rwc, binary.LittleEndian, &sz))
	setError(&err, binary.Read(rwc, binary.LittleEndian, plen))
	if err != nil {
		return err
	}
	buf := make([]byte, sz)
	n, err := rwc.Read(buf)
	if err != nil {
		return err
	}
	if n != int(sz) {
		return fmt.Errorf("Expected: %v. Got: %v\n", sz, n)
	}
	*method = string(buf)
	return nil
}

func (c *ccodec) WriteRequest(r *rpc.Request, body interface{}) error {
	if body == nil {
		return errors.New("Nil body")
	}

	query := body.(*Query)
	if err := writeHeader(c.rwc, r.Seq, r.ServiceMethod, query.d); err != nil {
		return err
	}

	n, err := c.rwc.Write(query.d)
	if n != len(query.d) {
		return errors.New("Unable to write payload.")
	}
	return err
}

func (c *ccodec) ReadResponseHeader(r *rpc.Response) error {
	if len(r.Error) > 0 {
		log.Fatal("client got response error: " + r.Error)
	}
	if err := parseHeader(c.rwc, &r.Seq,
		&r.ServiceMethod, &c.payloadLen); err != nil {
		return err
	}
	fmt.Println("Client got response:", r.Seq)
	fmt.Println("Client got response:", r.ServiceMethod)
	return nil
}

func (c *ccodec) ReadResponseBody(body interface{}) error {
	buf := make([]byte, c.payloadLen)
	n, err := c.rwc.Read(buf)
	if n != int(c.payloadLen) {
		return fmt.Errorf("Client expected: %d. Got: %d\n", c.payloadLen, n)
	}
	reply := body.(*Reply)
	reply.d = buf
	return err
}

func (c *ccodec) Close() error {
	return c.rwc.Close()
}
