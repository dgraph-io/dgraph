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

package services

import (
	"testing"
)

// ------------------- Mock Services --------------------
type MockSrvcA struct {
	running bool
}

func (s *MockSrvcA) Start() error {
	s.running = true
	return nil
}
func (s *MockSrvcA) Stop() error {
	s.running = false
	return nil
}

type MockSrvcB struct {
	running bool
}

func (s *MockSrvcB) Start() error {
	s.running = true
	return nil
}
func (s *MockSrvcB) Stop() error {
	s.running = false
	return nil
}

type FakeService struct{}

func (s *FakeService) Start() error { return nil }
func (s *FakeService) Stop()        {}

// --------------------------------------------------------

func TestServiceRegistry_RegisterService(t *testing.T) {
	r := NewServiceRegistry()

	a1 := &MockSrvcA{}
	a2 := &MockSrvcA{}

	r.RegisterService(a1)
	r.RegisterService(a2)

	if len(r.serviceTypes) > 1 {
		t.Fatalf("should not allow services of the same type to be registered")
	}
}

func TestServiceRegistry_StartStopAll(t *testing.T) {
	r := NewServiceRegistry()

	a := &MockSrvcA{}
	b := &MockSrvcB{}

	r.RegisterService(a)
	r.RegisterService(b)

	r.StartAll()

	if a.running != true || b.running != true {
		t.Fatal("failed to start service")
	}

	r.StopAll()

	if a.running != false || b.running != false {
		t.Fatal("failed to stop service")
	}

}

func TestServiceRegistry_Get_Err(t *testing.T) {
	r := NewServiceRegistry()

	a := &MockSrvcA{}
	b := &MockSrvcB{}

	r.RegisterService(a)
	r.RegisterService(b)

	if r.Get(a) == nil {
		t.Fatalf("Failed to fetch service: %T", a)
	}
	if err := r.Get(a).Start(); err != nil {
		t.Fatal(err)
	}

	if r.Get(b) == nil {
		t.Fatalf("Failed to fetch service: %T", b)
	}
	if err := r.Get(b).Start(); err != nil {
		t.Fatal(err)
	}

	f := &FakeService{}
	if s := r.Get(f); s != nil {
		t.Fatalf("Expected nil. Fetched service: %T", s)
	}
}
