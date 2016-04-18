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
	"bytes"
	"flag"
	"net"

	"github.com/dgraph-io/dgraph/query/protocolbuffer"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/protobuf/proto"
)

var glog = x.Log("client")

var port = flag.String("port", "8090", "Port to communicate with server")

func main() {

	// TODO(pawan) - Remove hardcoded query. Give helper methods to user for building query.
	var q0 = `{
    me(_xid_: m.06pj8) {
        type.object.name.en
        film.director.film {
            type.object.name.en
            film.film.starring {
                film.performance.character {
                    type.object.name.en
                }
                film.performance.actor {
                    type.object.name.en
                    film.director.film {
                        type.object.name.en
                    }
                }
            }
            film.film.initial_release_date
            film.film.country
            film.film.genre {
                type.object.name.en
            }
        }
    }
}`

	// TODO(pawan): Pick address for server from config
	conn, err := net.Dial("tcp", "127.0.0.1:"+*port)
	if err != nil {
		x.Err(glog, err).Fatal("DialTCPConnection")
	}

	_, err = conn.Write([]byte(q0))
	if err != nil {
		x.Err(glog, err).Fatal("Error in writing to server")
	}

	// TODO(pawan): Discuss and implement a better way of doing this.
	reply := make([]byte, 4096)
	_, err = conn.Read(reply)
	if err != nil {
		x.Err(glog, err).Fatal("Error in reading response from server")
	}
	// Trimming null bytes
	reply = bytes.Trim(reply, "\000")

	usg := &protocolbuffer.SubGraph{}
	if err := proto.Unmarshal(reply, usg); err != nil {
		x.Err(glog, err).Fatal("Error in umarshalling protocol buffer")
	}

	conn.Close()
}
