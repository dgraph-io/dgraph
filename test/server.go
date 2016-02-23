package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net/rpc"
	"reflect"
)

type scodec struct {
	rwc        io.ReadWriteCloser
	ebuf       *bufio.Writer
	payloadLen int32
}

func (c *scodec) ReadRequestHeader(r *rpc.Request) error {
	var err error
	if err = parseHeader(c.rwc, &r.Seq,
		&r.ServiceMethod, &c.payloadLen); err != nil {
		return err
	}

	fmt.Println("server using custom codec to read header")
	fmt.Println("server method called:", r.ServiceMethod)
	fmt.Println("server method called:", r.Seq)
	return nil
}

func (c *scodec) ReadRequestBody(data interface{}) error {
	if data == nil {
		log.Fatal("Why is data nil here?")
	}
	value := reflect.ValueOf(data)
	if value.Type().Kind() != reflect.Ptr {
		log.Fatal("Should of of type pointer")
	}

	b := make([]byte, c.payloadLen)
	n, err := c.rwc.Read(b)
	fmt.Printf("Worker read n bytes: %v %s\n", n, string(b))
	if err != nil {
		log.Fatal("server", err)
	}
	if n != int(c.payloadLen) {
		return errors.New("Server unable to read request.")
	}

	query := data.(*Query)
	query.d = b
	return nil
}

func (c *scodec) WriteResponse(resp *rpc.Response, data interface{}) error {
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

	if err := writeHeader(c.rwc, resp.Seq,
		resp.ServiceMethod, reply.d); err != nil {
		return err
	}

	_, err := c.rwc.Write(reply.d)
	return err
}

func (c *scodec) Close() error {
	return c.rwc.Close()
}
