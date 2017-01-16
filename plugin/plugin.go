package plugin

import (
	"net/http"
)

var (
	queryHandlers []func(*http.Request) (string, error)
	loader []func
)

func init() {
	queryHandlers = make([]func(*http.Request) (string, error), 0, 5)
}

func AddQueryHandler(f func(*http.Request) (string, error)) {
	queryHandlers = append(queryHandlers, f)
}

func AddClient() {

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
