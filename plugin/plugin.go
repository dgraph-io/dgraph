package plugin

import (
	"bytes"
	"net/http"

	"github.com/dgraph-io/dgraph/x"
)

var (
	queryHandlers = make([]func(*http.Request) (string, error), 0)
	keyPrefixes   = make([]KeyPrefix, 0)

	// For client.
	clientInits = make([]func() (string, error), 0)
)

func AddQueryHandler(f func(*http.Request) (string, error)) {
	queryHandlers = append(queryHandlers, f)
}

func AddClientInit(f func() (string, error)) {
	clientInits = append(clientInits, f)
}

func AddKeyPrefix(kp KeyPrefix) {
	keyPrefixes = append(keyPrefixes, kp)
}

func RunQueryHandlers(r *http.Request) ([]string, error) {
	var out []string
	for _, f := range queryHandlers {
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

// KeyPrefix adds a prefix to the keys. We expect these prefixes to be short.
type KeyPrefix interface {
	Data(attr string, uid uint64, pluginContext string) string
	Index(attr, term, pluginContext string) string
	Reverse(attr string, uid uint64, pluginContext string) string
}

// DataKeyPrefix generates a prefix from plugins.
func DataKeyPrefix(attr string, uid uint64, pluginContexts []string) string {
	// For now, we assume all plugins modify key prefixes. This can be easily
	// generalized. For example, pluginContexts can contain a few string arrays.
	x.AssertTruef(len(pluginContexts) == len(keyPrefixes), "%d vs %d",
		len(pluginContexts), len(keyPrefixes))
	var buf bytes.Buffer
	for i, m := range keyPrefixes {
		_, err := buf.WriteString(m.Data(attr, uid, pluginContexts[i]))
		x.Check(err)
	}
	return buf.String()
}

// IndexKeyPrefix generates a prefix from plugins.
func IndexKeyPrefix(attr, term string, pluginContexts []string) string {
	// For now, we assume all plugins modify key prefixes. This can be easily
	// generalized. For example, pluginContexts can contain a few string arrays.
	x.AssertTruef(len(pluginContexts) == len(keyPrefixes), "%d vs %d",
		len(pluginContexts), len(keyPrefixes))
	var buf bytes.Buffer
	for i, m := range keyPrefixes {
		_, err := buf.WriteString(m.Index(attr, term, pluginContexts[i]))
		x.Check(err)
	}
	return buf.String()
}

// ReverseKeyPrefix generates a prefix from plugins.
func ReverseKeyPrefix(attr string, uid uint64, pluginContexts []string) string {
	// For now, we assume all plugins modify key prefixes. This can be easily
	// generalized. For example, pluginContexts can contain a few string arrays.
	x.AssertTruef(len(pluginContexts) == len(keyPrefixes), "%d vs %d",
		len(pluginContexts), len(keyPrefixes))
	var buf bytes.Buffer
	for i, m := range keyPrefixes {
		_, err := buf.WriteString(m.Reverse(attr, uid, pluginContexts[i]))
		x.Check(err)
	}
	return buf.String()
}
