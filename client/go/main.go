/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"flag"
	"fmt"
	"net"

	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("client")

var port = flag.String("port", "3000", "Port to communicate with server")

func main() {

	var q0 = `
  {
    user(_xid_:alice) {
      follows {
        _xid_
        status
      }
      _xid_
      status
    }
  }
`

	conn, err := net.Dial("tcp", "127.0.0.1:"+*port)
	if err != nil {
		glog.Fatalf("While running server: %v", err)
	}

	fmt.Println("sending data", []byte(q0))
	_, err = conn.Write([]byte(q0))
	if err != nil {
		x.Err(glog, err).Fatal("Error in writing to server")
	}

	reply := []byte{}
	_, err = conn.Read(reply)
	if err != nil {
		x.Err(glog, err).Fatal("Error in reading response from server")
	}
	fmt.Println(string(reply))

	conn.Close()

}
