package services

import "testing"

// ------------------- Mock Services --------------------
type MockSrvcA struct {
	running bool
}

func (s *MockSrvcA) Start() <-chan error {
	s.running = true
	return make(chan error)
}
func (s *MockSrvcA) Stop() {
	s.running = false
}

type MockSrvcB struct {
	running bool
}

func (s *MockSrvcB) Start() <-chan error {
	s.running = true
	return make(chan error)
}
func (s *MockSrvcB) Stop() {
	s.running = false
}

type FakeService struct{}

func (s *FakeService) Start() <-chan error { return *new(<-chan error) }
func (s *FakeService) Stop()               {}

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

func TestServiceRegistry_StartAll(t *testing.T) {
	r := NewServiceRegistry()

	a := &MockSrvcA{}
	b := &MockSrvcB{}

	r.RegisterService(a)
	r.RegisterService(b)

	r.StartAll()

	if a.running != true || b.running != true {
		t.Fatal("failed to start service")
	}
}

func TestServiceRegistry_Get_Err(t *testing.T) {
	r := NewServiceRegistry()

	a := &MockSrvcA{}
	b := &MockSrvcB{}

	r.RegisterService(a)
	r.RegisterService(b)

	r.StartAll()

	if r.Get(a) == nil || r.Err(a) == nil {
		t.Fatalf("Failed to fetch service: %T", a)
	}
	if r.Get(b) == nil || r.Err(a) == nil {
		t.Fatalf("Failed to fetch service: %T", b)
	}

	f := &FakeService{}
	if s := r.Get(f); s != nil {
		t.Fatalf("Expected nil. Fetched service: %T", s)
	}
	if e := r.Err(f); e != nil {
		t.Fatalf("Expected nil. Got: %T", e)
	}
}
