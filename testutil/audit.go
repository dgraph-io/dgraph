package testutil

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func VerifyLogs(t *testing.T, path string, cmds []string) {
	abs, err := filepath.Abs(path)
	require.Nil(t, err)
	f, err := os.Open(abs)
	require.Nil(t, err)

	type log struct {
		Msg string `json:"endpoint"`
	}
	logMap := make(map[string]bool)

	fileScanner := bufio.NewScanner(f)
	for fileScanner.Scan() {
		bytes := fileScanner.Bytes()
		l := new(log)
		_ = json.Unmarshal(bytes, l)
		logMap[l.Msg] = true
	}
	for _, m := range cmds {
		if !logMap[m] {
			t.Fatalf("audit logs not present for command %s", m)
		}
	}
}
