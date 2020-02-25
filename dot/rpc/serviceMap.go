// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package rpc

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type service struct {
	name     string
	rcvr     reflect.Value
	rcvrType reflect.Type
	methods  map[string]*serviceMethod
}

type serviceMethod struct {
	method reflect.Method
	alias  string
	// TODO: I think these can be removed if we only use 1 args/reply type
	argsType  reflect.Type
	replyType reflect.Type
}

type serviceMap struct {
	mutex    sync.Mutex
	services map[string]*service
}

// Precompute these for efficiency
var typeOfError = reflect.TypeOf((*error)(nil)).Elem()
var typeOfRequest = reflect.TypeOf((*http.Request)(nil)).Elem()

func (m *serviceMap) register(rcvr interface{}, name string) error {
	if name == "" {
		return fmt.Errorf("unspecified name, please specify a name")
	}

	if m.services[name] != nil {
		return fmt.Errorf("name is already registered")
	}

	s := &service{
		name:     name,
		rcvr:     reflect.ValueOf(rcvr),
		rcvrType: reflect.TypeOf(rcvr),
		methods:  make(map[string]*serviceMethod),
	}

	for i := 0; i < s.rcvrType.NumMethod(); i++ {
		method := s.rcvrType.Method(i)
		methodType := method.Type

		if method.PkgPath != "" {
		}
		// Must have receiver and 3 inputs
		if methodType.NumIn() != 4 {
		}
		// First arg must be http.Request
		reqType := methodType.In(1)
		if reqType.Kind() != reflect.Ptr || reqType.Elem() != typeOfRequest {
		}
		// Second arg must be pointer and exported
		argsType := methodType.In(2)
		if argsType.Kind() != reflect.Ptr || !isExported(method.Name) || isBuiltin(argsType) {
		}
		// Third arg must be pointer and exported
		replyType := methodType.In(3)
		if replyType.Kind() != reflect.Ptr || !isExported(method.Name) || isBuiltin(replyType) {
		}
		// Must have 1 return value of type error
		if methodType.NumOut() != 1 {
		}
		returnType := methodType.Out(0)
		if returnType != typeOfError {
		}
		// Force first character to lower case
		methodRunes := []rune(method.Name)
		methodAlias := string(unicode.ToLower(methodRunes[0])) + string(methodRunes[1:])

		m.mutex.Lock()
		s.methods[methodAlias] = &serviceMethod{
			method:    method,
			alias:     methodAlias,
			argsType:  argsType.Elem(),
			replyType: replyType.Elem(),
		}
		m.mutex.Unlock()
	}

	if len(s.methods) == 0 {
		return fmt.Errorf("rpc: expected some methods to register for service %s, got none", s.name)
	}

	if m.services == nil {
		m.services = make(map[string]*service)
	}

	m.services[s.name] = s
	return nil
}

func (m *serviceMap) get(id string) (*service, *serviceMethod, error) {
	tokens := strings.Split(id, "_")
	if len(tokens) != 2 {
		err := fmt.Errorf("invalid method name: %s", id)
		return nil, nil, err
	}
	m.mutex.Lock()
	service := m.services[tokens[0]]
	m.mutex.Unlock()
	if service == nil {
		err := fmt.Errorf("service %s not recognized", tokens[0])
		return nil, nil, err
	}
	// Force first character to lower case
	methodRunes := []rune(tokens[1])
	methodAlias := string(unicode.ToLower(methodRunes[0])) + string(methodRunes[1:])
	method := service.methods[methodAlias]
	if method == nil {
		err := fmt.Errorf("method %s in service %s not recognized", tokens[0], tokens[1])
		return nil, nil, err
	}
	return service, method, nil
}

func isExported(name string) bool {
	firstRune, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(firstRune)
}

func isBuiltin(t reflect.Type) bool {
	// Follow indirection
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Should be empty if builtin
	return t.PkgPath() == ""
}
