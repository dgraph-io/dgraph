package plugin

import (
	"bytes"
	"net/http"

	"github.com/dgraph-io/dgraph/x"
)

var (
	contextBuilders = make([]func(*http.Request) (string, error), 0)
	keyPrefixes     = make([]KeyPrefix, 0)
	prefixLen       int

	// For client.
	clientInits = make([]func() (string, error), 0)
)

// PrefixLen returns total prefix length.
func PrefixLen() int { return prefixLen }

func AddContextBuilder(f func(*http.Request) (string, error)) {
	contextBuilders = append(contextBuilders, f)
}

func AddClientInit(f func() (string, error)) {
	clientInits = append(clientInits, f)
}

func AddKeyPrefix(kp KeyPrefix) {
	keyPrefixes = append(keyPrefixes, kp)
	prefixLen += kp.Length()
}

func Contexts(r *http.Request) ([]string, error) {
	var out []string
	for _, f := range contextBuilders {
		s, err := f(r)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func ClientInit() ([]string, error) {
	var out []string
	for _, f := range clientInits {
		s, err := f()
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// KeyPrefix adds a prefix to the keys. For now, we assume fixed width.
type KeyPrefix interface {
	Prefix(pluginContext string) string
	Length() int // Assume fixed length for now. It makes keys.Parse more efficient.
}

// Prefix returns prefixes of all plugins.
func Prefix(pluginContexts []string) []byte {
	// For now, we assume all plugins modify key prefixes. This can be easily
	// generalized. For example, pluginContexts can contain a few string arrays.
	x.AssertTruef(len(pluginContexts) == len(keyPrefixes), "%d vs %d",
		len(pluginContexts), len(keyPrefixes))
	var buf bytes.Buffer
	for i, m := range keyPrefixes {
		x.Check2(buf.WriteString(m.Prefix(pluginContexts[i])))
	}
	return buf.Bytes()
}
