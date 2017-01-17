package plugin

import (
	"bytes"
	"net/http"

	"github.com/dgraph-io/dgraph/x"
)

var (
	inits           = make([]func(bool) (string, error), 0)
	contextBuilders = make([]func(*http.Request) (string, error), 0)
	keyPrefixes     = make([]KeyPrefix, 0)
	prefixLen       int
)

// Init loads all plugins. If isClient, we return pluginContexts for client.
func Init(isClient bool) ([]string, error) {
	var out []string
	for _, f := range inits {
		s, err := f(isClient)
		if err != nil {
			return nil, err
		}
		if isClient {
			out = append(out, s)
		}
	}
	return out, nil
}

// PrefixLen returns total prefix length.
func PrefixLen() int { return prefixLen }

// AddContextBuilder adds a context builder. Call this in your plugin Init function.
func AddContextBuilder(f func(*http.Request) (string, error)) {
	contextBuilders = append(contextBuilders, f)
}

// AddInit adds a init function for a plugin.
func AddInit(f func(bool) (string, error)) {
	inits = append(inits, f)
}

// AddKeyPrefix adds a KeyPrefix function.
func AddKeyPrefix(kp KeyPrefix) {
	keyPrefixes = append(keyPrefixes, kp)
	prefixLen += kp.Length()
}

// Contexts build pluginContexts from HTTP request. It uses list of context builders.
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

// KeyPrefix adds a prefix to the keys. For now, we assume fixed width.
type KeyPrefix interface {
	Prefix(pluginContext string) string
	Length() int // Assume fixed length for now. It makes keys.Parse more efficient.
}

// Prefix returns prefixes of all plugins.
func Prefix(pluginContexts []string) string {
	// For now, we assume all plugins modify key prefixes. This can be easily
	// generalized. For example, pluginContexts can contain a few string arrays.
	x.AssertTruef(len(pluginContexts) == len(keyPrefixes), "%d vs %d",
		len(pluginContexts), len(keyPrefixes))
	var buf bytes.Buffer
	for i, m := range keyPrefixes {
		x.Check2(buf.WriteString(m.Prefix(pluginContexts[i])))
	}
	return buf.String()
}
