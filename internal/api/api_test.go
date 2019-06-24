package api

import "testing"

// -------------- Mock Apis ------------------
const (
	TestPeerCount = 1337
	TestVersion   = "1.2.3"
)

type MockP2pApi struct{}

func (a *MockP2pApi) PeerCount() int {
	return TestPeerCount
}

type MockRuntimeApi struct{}

func (a *MockRuntimeApi) Version() string {
	return TestVersion
}

// -------------------------------------------

func TestSystemModule(t *testing.T) {
	srvc := NewApiService(&MockP2pApi{}, &MockRuntimeApi{})

	// System.PeerCount
	c := srvc.Api.System.PeerCount()
	if c != TestPeerCount {
		t.Fatalf("System.PeerCount - expected: %d got: %d\n", TestPeerCount, c)
	}

	// System.Version
	v := srvc.Api.System.Version()
	if v != TestVersion {
		t.Fatalf("System.Version - expected: %s got: %s\n", TestVersion, v)
	}
}
