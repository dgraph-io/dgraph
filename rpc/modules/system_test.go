package modules

import (
	"testing"

	"github.com/ChainSafe/gossamer/internal/api"
)

var (
	testRuntimeVersion = "1.2.3"
)

type mockruntimeApi struct{}

func (a *mockruntimeApi) Version() string {
	return testRuntimeVersion
}

func newMockApi() *api.Api {
	runtimeApi := &mockruntimeApi{}

	return &api.Api{
		System: api.NewSystemModule(nil, runtimeApi),
	}
}

func TestSystemModule_Version(t *testing.T) {
	sys := NewSystemModule(newMockApi())

	vres := &SystemVersionResponse{}
	err := sys.Version(nil, nil, vres)
	if err != nil {
		t.Fatal(err)
	}
	if vres.Version != testRuntimeVersion {
		t.Fatalf("System.Version: expected: %s got: %s\n", vres.Version, testRuntimeVersion)
	}
}
