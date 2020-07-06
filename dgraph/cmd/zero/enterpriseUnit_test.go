package zero

import (
	"testing"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseLicenseUnit(t *testing.T) {

	expiredKey := "enterpriseExperiedKey.Key"
	invalidKey := "enterpriseInvalidKey.Key"

	var tests = []struct {
		name           string
		licenseKeyPath string
		expectError    bool
		expectedOutput string
	}{
		{
			"Using expired entrerprised license key should return an error",
			expiredKey,
			true,
			`while extracting enterprise details from the license: while decoding license file: EOF`,
		},
		{
			"Using invalid entrerprised license key should return an error",
			invalidKey,
			true,
			`while extracting enterprise details from the license: while decoding license file: EOF`,
		},
	}
	server := &Server{
		state: &pb.MembershipState{
			Groups: map[uint32]*pb.Group{1: {Members: map[uint64]*pb.Member{}}},
		},
	}
	for _, tt := range tests {
		t.Logf("Running: %s\n", tt.name)
		err := server.applyLicenseFileReturnError(expiredKey)
		if tt.expectError {
			require.Error(t, err)
			continue
		}

		require.NoError(t, err)
	}

}
