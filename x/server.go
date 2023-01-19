package x

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/soheilhy/cmux"

	"github.com/dgraph-io/ristretto/z"
)

var (
	// ServerCloser is used to signal and wait for other goroutines to return gracefully after user
	// requests shutdown.
	ServerCloser = z.NewCloser(0)
)

func StartListenHttpAndHttps(l net.Listener, tlsCfg *tls.Config, closer *z.Closer) {
	defer closer.Done()
	m := cmux.New(l)
	startServers(m, tlsCfg)
	err := m.Serve()
	if err != nil {
		glog.Errorf("error from cmux serve: %v", err)
	}
}

func startServers(m cmux.CMux, tlsConf *tls.Config) {
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
		if strings.HasPrefix(path, "/health") {
			return true
		}
		return false
	})
	go startListen(httpRule)

	// if tls is enabled, make tls encryption based connections as default
	if tlsConf != nil {
		httpsRule := m.Match(cmux.Any())
		// this is chained listener. tls listener will decrypt
		// the message and send it in plain text to HTTP server
		go startListen(tls.NewListener(httpsRule, tlsConf))
	}
}

func startListen(l net.Listener) {
	srv := &http.Server{
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
