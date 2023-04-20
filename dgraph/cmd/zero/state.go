package zero

import (
	"net"
	"net/http"
	"os"
	"os/signal"

	"github.com/dgraph-io/dgraph/ee/audit"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/spf13/viper"
	"go.opencensus.io/zpages"
)

func (st *State) InitServers(conf *viper.Viper, grpcListener net.Listener, httpListener net.Listener, store *raftwal.DiskStorage) {
	st.serveGRPC(conf, grpcListener, store)

	tlsCfg, err := x.LoadServerTLSConfig(Zero.Conf)
	x.Check(err)
	go x.StartListenHttpAndHttps(httpListener, tlsCfg, st.zero.closer)
}

func (st *State) RegisterHttpHandlers(limit *z.SuperFlag) {
	baseMux := http.NewServeMux()
	http.Handle("/", audit.AuditRequestHttp(baseMux))

	baseMux.HandleFunc("/health", st.pingResponse)
	// the following endpoints are disabled only if the flag is explicitly set to true
	if !limit.GetBool("disable-admin-http") {
		baseMux.HandleFunc("/state", st.getState)
		baseMux.HandleFunc("/removeNode", st.removeNode)
		baseMux.HandleFunc("/moveTablet", st.moveTablet)
		baseMux.HandleFunc("/assign", st.assign)
		baseMux.HandleFunc("/enterpriseLicense", st.applyEnterpriseLicense)
	}
	baseMux.HandleFunc("/debug/jemalloc", x.JemallocHandler)
	zpages.Handle(baseMux, "/debug/z")
}

func (st *State) InitAndStartNode() {
	x.Check(st.node.initAndStartNode())
}

func (st *State) PeriodicallyPostTelemetry() {
	st.zero.periodicallyPostTelemetry()
}

func (st *State) HandleShutDown(sdCh chan os.Signal, grpcListener net.Listener, httpListener net.Listener) {
	st.zero.closer.AddRunning(1)

	go func() {
		defer st.zero.closer.Done()
		<-st.zero.closer.HasBeenClosed()
		glog.Infoln("Shutting down...")
		close(sdCh)
		// Close doesn't close already opened connections.

		// Stop all HTTP requests.
		_ = httpListener.Close()
		// Stop Raft.
		st.node.closer.SignalAndWait()
		// Stop all internal requests.
		_ = grpcListener.Close()

		x.RemoveCidFile()
	}()
}

func (st *State) MonitorMetrics() {
	st.zero.closer.AddRunning(2)
	go x.MonitorMemoryMetrics(st.zero.closer)
	go x.MonitorDiskMetrics("wal_fs", Opts.W, st.zero.closer)
}

func (st *State) Wait() {
	glog.Infoln("Running Dgraph Zero...")
	st.zero.closer.Wait()
	glog.Infoln("Closer closed.")
}

func (st *State) Close(store *raftwal.DiskStorage) {
	err := store.Close()
	glog.Infof("Raft WAL closed with err: %v\n", err)

	audit.Close()

	st.zero.orc.close()
	glog.Infoln("All done. Goodbye!")
}

func (st *State) HandleSignals(sdCh chan os.Signal) {
	var sigCnt int
	for sig := range sdCh {
		glog.Infof("--- Received %s signal", sig)
		sigCnt++
		if sigCnt == 1 {
			signal.Stop(sdCh)
			st.zero.closer.Signal()
		} else if sigCnt == 3 {
			glog.Infof("--- Got interrupt signal 3rd time. Aborting now.")
			os.Exit(1)
		} else {
			glog.Infof("--- Ignoring interrupt signal.")
		}
	}
}