package zero

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/jsonpb"
)

// intFromQueryParam checks for name as a query param, converts it to uint64 and returns it.
// It also writes any errors to w. A bool is returned to indicate if the param was parsed
// successfully.
func intFromQueryParam(w http.ResponseWriter, r *http.Request, name string) (uint64, bool) {
	str := r.URL.Query().Get(name)
	if len(str) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest, fmt.Sprintf("%s not passed", name))
		return 0, false
	}
	val, err := strconv.ParseUint(str, 0, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest, fmt.Sprintf("Error while parsing %s", name))
		return 0, false
	}
	return val, true
}

// removeNode can be used to remove a node from the cluster. It takes in the RAFT id of the node
// and the group it belongs to. It can be used to remove Dgraph server and Zero nodes(group=0).
func (st *state) removeNode(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	nodeId, ok := intFromQueryParam(w, r, "id")
	if !ok {
		return
	}
	groupId, ok := intFromQueryParam(w, r, "group")
	if !ok {
		return
	}

	if err := st.zero.removeNode(context.Background(), nodeId, uint32(groupId)); err != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	w.Write([]byte(fmt.Sprintf("Removed node with group: %v, idx: %v", groupId, nodeId)))
}

// moveTablet can be used to move a tablet to a specific group. It takes in tablet and group as
// argument.
func (st *state) moveTablet(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	tablet := r.URL.Query().Get("tablet")
	if len(tablet) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest, "tablet is a mandatory query parameter")
		return
	}

	groupId, ok := intFromQueryParam(w, r, "group")
	if !ok {
		return
	}
	dstGroup := uint32(groupId)
	knownGroups := st.zero.KnownGroups()
	var isKnown bool
	for _, grp := range knownGroups {
		if grp == dstGroup {
			isKnown = true
			break
		}
	}
	if !isKnown {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest, fmt.Sprintf("Group: [%d] is not a known group.", dstGroup))
		return
	}

	tab := st.zero.ServingTablet(tablet)
	if tab == nil {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest, fmt.Sprintf("No tablet found for: %s", tablet))
		return
	}

	srcGroup := tab.GroupId
	if srcGroup == dstGroup {
		w.WriteHeader(http.StatusInternalServerError)
		x.SetStatus(w, x.ErrorInvalidRequest,
			fmt.Sprintf("Tablet: [%s] is already being served by group: [%d]", tablet, srcGroup))
		return
	}

	if err := st.zero.movePredicate(tablet, srcGroup, dstGroup); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		x.SetStatus(w, x.Error, err.Error())
		return
	}

	w.Write([]byte(fmt.Sprintf("Predicate: [%s] moved from group: [%d] to [%d]",
		tablet, srcGroup, dstGroup)))
}

func (st *state) getState(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	mstate := st.zero.membershipState()
	if mstate == nil {
		x.SetStatus(w, x.ErrorNoData, "No membership state found.")
		return
	}

	m := jsonpb.Marshaler{}
	if err := m.Marshal(w, mstate); err != nil {
		x.SetStatus(w, x.ErrorNoData, err.Error())
		return
	}
}

func (st *state) serveHTTP(l net.Listener, wg *sync.WaitGroup) {
	srv := &http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 600 * time.Second,
		IdleTimeout:  2 * time.Minute,
	}

	go func() {
		defer wg.Done()
		err := srv.Serve(l)
		log.Printf("Stopped taking more http(s) requests. Err: %s", err.Error())
		ctx, cancel := context.WithTimeout(context.Background(), 630*time.Second)
		defer cancel()
		err = srv.Shutdown(ctx)
		log.Printf("All http(s) requests finished.")
		if err != nil {
			log.Printf("Http(s) shutdown err: %v", err.Error())
		}
	}()
}
