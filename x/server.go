package x

import (
	"bufio"
	"context"
	"crypto/tls"
	"expvar"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/soheilhy/cmux"

	"github.com/dgraph-io/ristretto/v2/z"
)

var (
	// ServerCloser is used to signal and wait for other goroutines to return gracefully after user
	// requests shutdown.
	ServerCloser = z.NewCloser(0)
)

func StartListenHttpAndHttps(l net.Listener, tlsCfg *tls.Config, closer *z.Closer, h http.Handler) {
	defer closer.Done()
	m := cmux.New(l)
	startServers(m, tlsCfg, h)
	err := m.Serve()
	if err != nil {
		glog.Errorf("error from cmux serve: %v", err)
	}
}

func startServers(m cmux.CMux, tlsConf *tls.Config, h http.Handler) {
	httpRule := m.Match(func(r io.Reader) bool {
		// no tls config is provided. http is being used.
		if tlsConf == nil {
			return true
		}
		path, ok := parseRequestPath(r)
		if !ok {
			// not able to parse the request. Let it be resolved via TLS
			return false
		}
		// health endpoint will always be available over http.
		// This is necessary for orchestration. It needs to be worked for
		// monitoring tools which operate without authentication.
		return strings.HasPrefix(path, "/health")
	})
	go startListen(httpRule, h)

	// if tls is enabled, make tls encryption based connections as default
	if tlsConf != nil {
		httpsRule := m.Match(cmux.Any())
		// this is chained listener. tls listener will decrypt
		// the message and send it in plain text to HTTP server
		go startListen(tls.NewListener(httpsRule, tlsConf), h)
	}
}

func startListen(l net.Listener, h http.Handler) {
	srv := &http.Server{
		Handler:           h, // nil falls back to http.DefaultServeMux
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      600 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ReadHeaderTimeout: 60 * time.Second,
	}

	err := srv.Serve(l)
	glog.Errorf("Stopped taking more http(s) requests. Err: %v", err)
	ctx, cancel := context.WithTimeout(context.Background(), 630*time.Second)
	defer cancel()
	err = srv.Shutdown(ctx)
	glog.Infoln("All http(s) requests finished.")
	if err != nil {
		glog.Errorf("Http(s) shutdown err: %v", err)
	}
}

func parseRequestPath(r io.Reader) (path string, ok bool) {
	request, err := http.ReadRequest(bufio.NewReader(r))
	if err != nil {
		return "", false
	}

	return request.URL.Path, true
}

// filteredExpvarHandler serves /debug/vars but omits the "cmdline" key that
// expvar publishes by default (os.Args), which may contain the admin token
// from --security "token=...".
func filteredExpvarHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, "{\n")
	first := true
	expvar.Do(func(kv expvar.KeyValue) {
		if kv.Key == "cmdline" {
			return
		}
		if !first {
			fmt.Fprintf(w, ",\n")
		}
		first = false
		fmt.Fprintf(w, "%q: %s", kv.Key, kv.Value)
	})
	fmt.Fprintf(w, "\n}\n")
}

// SanitizedDefaultServeMux returns an http.Handler that wraps
// http.DefaultServeMux but blocks endpoints that expose the full process
// command line (which may include the admin token from --security "token=..."):
//   - /debug/pprof/cmdline — registered by net/http/pprof
//   - /debug/vars          — served with a filtered handler that omits "cmdline"
func SanitizedDefaultServeMux() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/debug/pprof/cmdline" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/debug/vars" {
			filteredExpvarHandler(w, r)
			return
		}
		http.DefaultServeMux.ServeHTTP(w, r)
	})
}
