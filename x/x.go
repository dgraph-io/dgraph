/*
 * Copyright 2015-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package x

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/trace"
	"golang.org/x/crypto/ssh/terminal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/status"
)

// Error constants representing different types of errors.
var (
	// ErrNotSupported is thrown when an enterprise feature is requested in the open source version.
	ErrNotSupported = errors.Errorf("Feature available only in Dgraph Enterprise Edition")
)

const (
	// Success is equivalent to the HTTP 200 error code.
	Success = "Success"
	// ErrorUnauthorized is equivalent to the HTTP 401 error code.
	ErrorUnauthorized = "ErrorUnauthorized"
	// ErrorInvalidMethod is equivalent to the HTTP 405 error code.
	ErrorInvalidMethod = "ErrorInvalidMethod"
	// ErrorInvalidRequest is equivalent to the HTTP 400 error code.
	ErrorInvalidRequest = "ErrorInvalidRequest"
	// Error is a general error code.
	Error = "Error"
	// ErrorNoData is an error returned when the requested data cannot be returned.
	ErrorNoData = "ErrorNoData"
	// ValidHostnameRegex is a regex that accepts our expected hostname format.
	ValidHostnameRegex = "^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\\-]*[a-zA-Z0-9])\\.)*([A-Za-z0-9]" +
		"|[A-Za-z0-9][A-Za-z0-9\\-]*[A-Za-z0-9])$"
	// Star is equivalent to using * in a mutation.
	// When changing this value also remember to change in in client/client.go:DeleteEdges.
	Star = "_STAR_ALL"

	// GrpcMaxSize is the maximum possible size for a gRPC message.
	// Dgraph uses the maximum size for the most flexibility (4GB - equal
	// to the max grpc frame size). Users will still need to set the max
	// message sizes allowable on the client size when dialing.
	GrpcMaxSize = 4 << 30

	// PortZeroGrpc is the default gRPC port for zero.
	PortZeroGrpc = 5080
	// PortZeroHTTP is the default HTTP port for zero.
	PortZeroHTTP = 6080
	// PortInternal is the default port for internal use.
	PortInternal = 7080
	// PortHTTP is the default HTTP port for alpha.
	PortHTTP = 8080
	// PortGrpc is the default gRPC port for alpha.
	PortGrpc = 9080
	// ForceAbortDifference is the maximum allowed difference between
	// AppliedUntil - TxnMarks.DoneUntil() before old transactions start getting aborted.
	ForceAbortDifference = 5000

	// FacetDelimeter is the symbol used to distinguish predicate names from facets.
	FacetDelimeter = "|"

	// GrootId is the ID of the admin user for ACLs.
	GrootId = "groot"
	// AclPredicates is the JSON representation of the predicates reserved for use
	// by the ACL system.
	AclPredicates = `
{"predicate":"dgraph.xid","type":"string", "index": true, "tokenizer":["exact"], "upsert": true},
{"predicate":"dgraph.password","type":"password"},
{"predicate":"dgraph.user.group","list":true, "reverse": true, "type": "uid"},
{"predicate":"dgraph.group.acl","type":"string"}
`
)

var (
	// Useful for running multiple servers on the same machine.
	regExpHostName = regexp.MustCompile(ValidHostnameRegex)
	// Nilbyte is a nil byte slice. Used
	Nilbyte []byte
)

// ShouldCrash returns true if the error should cause the process to crash.
func ShouldCrash(err error) bool {
	if err == nil {
		return false
	}
	errStr := status.Convert(err).Message()
	return strings.Contains(errStr, "REUSE_RAFTID") ||
		strings.Contains(errStr, "REUSE_ADDR") ||
		strings.Contains(errStr, "NO_ADDR") ||
		strings.Contains(errStr, "ENTERPRISE_LIMIT_REACHED")
}

// WhiteSpace Replacer removes spaces and tabs from a string.
var WhiteSpace = strings.NewReplacer(" ", "", "\t", "")

// GqlError is a GraphQL spec compliant error structure.  See GraphQL spec on
// errors here: https://graphql.github.io/graphql-spec/June2018/#sec-Errors
//
// Note: "Every error must contain an entry with the key message with a string
// description of the error intended for the developer as a guide to understand
// and correct the error."
//
// "If an error can be associated to a particular point in the request [the error]
// should contain an entry with the key locations with a list of locations"
//
// Path is about GraphQL results and Errors for GraphQL layer.
//
// Extensions is for everything else.
type GqlError struct {
	Message    string                 `json:"message"`
	Locations  []Location             `json:"locations,omitempty"`
	Path       []interface{}          `json:"path,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// A Location is the Line+Column index of an error in a request.
type Location struct {
	Line   int `json:"line,omitempty"`
	Column int `json:"column,omitempty"`
}

type queryRes struct {
	Errors []GqlError `json:"errors"`
}

// SetStatus sets the error code, message and the newly assigned uids
// in the http response.
func SetStatus(w http.ResponseWriter, code, msg string) {
	var qr queryRes
	ext := make(map[string]interface{})
	ext["code"] = code
	qr.Errors = append(qr.Errors, GqlError{Message: msg, Extensions: ext})
	if js, err := json.Marshal(qr); err == nil {
		if _, err := w.Write(js); err != nil {
			glog.Errorf("Error while writing: %+v", err)
		}
	} else {
		panic(fmt.Sprintf("Unable to marshal: %+v", qr))
	}
}

// SetHttpStatus is similar to SetStatus but sets a proper HTTP status code
// in the response instead of always returning HTTP 200 (OK).
func SetHttpStatus(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	SetStatus(w, "error", msg)
}

// AddCorsHeaders adds the CORS headers to an HTTP response.
func AddCorsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "X-Dgraph-AccessToken, "+
		"Content-Type, Content-Length, Accept-Encoding, Cache-Control, "+
		"X-CSRF-Token, X-Auth-Token, X-Requested-With")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Connection", "close")
}

// QueryResWithData represents a response that holds errors as well as data.
type QueryResWithData struct {
	Errors []GqlError `json:"errors"`
	Data   *string    `json:"data"`
}

// SetStatusWithData sets the errors in the response and ensures that the data key
// in the data is present with value nil.
// In case an error was encountered after the query execution started, we have to return data
// key with null value according to GraphQL spec.
func SetStatusWithData(w http.ResponseWriter, code, msg string) {
	var qr QueryResWithData
	ext := make(map[string]interface{})
	ext["code"] = code
	qr.Errors = append(qr.Errors, GqlError{Message: msg, Extensions: ext})
	// This would ensure that data key is present with value null.
	if js, err := json.Marshal(qr); err == nil {
		if _, err := w.Write(js); err != nil {
			glog.Errorf("Error while writing: %+v", err)
		}
	} else {
		panic(fmt.Sprintf("Unable to marshal: %+v", qr))
	}
}

// Reply sets the body of an HTTP response to the JSON representation of the given reply.
func Reply(w http.ResponseWriter, rep interface{}) {
	if js, err := json.Marshal(rep); err == nil {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, string(js))
	} else {
		SetStatus(w, Error, "Internal server error")
	}
}

// ParseRequest parses the body of the given request.
func ParseRequest(w http.ResponseWriter, r *http.Request, data interface{}) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&data); err != nil {
		SetStatus(w, Error, fmt.Sprintf("While parsing request: %v", err))
		return false
	}
	return true
}

// Min returns the minimum of the two given numbers.
func Min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// Max returns the maximum of the two given numbers.
func Max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// RetryUntilSuccess runs the given function until it succeeds or can no longer be retried.
func RetryUntilSuccess(maxRetries int, waitAfterFailure time.Duration,
	f func() error) error {
	var err error
	for retry := maxRetries; retry != 0; retry-- {
		if err = f(); err == nil {
			return nil
		}
		if waitAfterFailure > 0 {
			time.Sleep(waitAfterFailure)
		}
	}
	return err
}

// HasString returns whether the slice contains the given string.
func HasString(a []string, b string) bool {
	for _, k := range a {
		if k == b {
			return true
		}
	}
	return false
}

// ReadLine reads a single line from a buffered reader. The line is read into the
// passed in buffer to minimize allocations. This is the preferred
// method for loading long lines which could be longer than the buffer
// size of bufio.Scanner.
func ReadLine(r *bufio.Reader, buf *bytes.Buffer) error {
	isPrefix := true
	var err error
	buf.Reset()
	for isPrefix && err == nil {
		var line []byte
		// The returned line is an pb.buffer in bufio and is only
		// valid until the next call to ReadLine. It needs to be copied
		// over to our own buffer.
		line, isPrefix, err = r.ReadLine()
		if err == nil {
			if _, err := buf.Write(line); err != nil {
				return err
			}
		}
	}
	return err
}

// FixedDuration returns the given duration as a string of fixed length.
func FixedDuration(d time.Duration) string {
	str := fmt.Sprintf("%02ds", int(d.Seconds())%60)
	if d >= time.Minute {
		str = fmt.Sprintf("%02dm", int(d.Minutes())%60) + str
	}
	if d >= time.Hour {
		str = fmt.Sprintf("%02dh", int(d.Hours())) + str
	}
	return str
}

// PageRange returns start and end indices given pagination params. Note that n
// is the size of the input list.
func PageRange(count, offset, n int) (int, int) {
	if n == 0 {
		return 0, 0
	}
	if count == 0 && offset == 0 {
		return 0, n
	}
	if count < 0 {
		// Items from the back of the array, like Python arrays. Do a positive mod n.
		if count*-1 > n {
			count = -n
		}
		return (((n + count) % n) + n) % n, n
	}
	start := offset
	if start < 0 {
		start = 0
	}
	if start > n {
		return n, n
	}
	if count == 0 { // No count specified. Just take the offset parameter.
		return start, n
	}
	end := start + count
	if end > n {
		end = n
	}
	return start, end
}

// ValidateAddress checks whether given address can be used with grpc dial function
func ValidateAddress(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if p, err := strconv.Atoi(port); err != nil || p <= 0 || p >= 65536 {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	// try to parse as hostname as per hostname RFC
	if len(strings.Replace(host, ".", "", -1)) > 255 {
		return false
	}
	return regExpHostName.MatchString(host)
}

// RemoveDuplicates sorts the slice of strings and removes duplicates. changes the input slice.
// This function should be called like: someSlice = RemoveDuplicates(someSlice)
func RemoveDuplicates(s []string) (out []string) {
	sort.Strings(s)
	out = s[:0]
	for i := range s {
		if i > 0 && s[i] == s[i-1] {
			continue
		}
		out = append(out, s[i])
	}
	return
}

// BytesBuffer provides a buffer backed by byte slices.
type BytesBuffer struct {
	data [][]byte
	off  int
	sz   int
}

func (b *BytesBuffer) grow(n int) {
	if n < 128 {
		n = 128
	}
	if len(b.data) == 0 {
		b.data = append(b.data, make([]byte, n))
	}

	last := len(b.data) - 1
	// Return if we have sufficient space
	if len(b.data[last])-b.off >= n {
		return
	}
	sz := len(b.data[last]) * 2
	if sz > 512<<10 {
		sz = 512 << 10 // 512 KB
	}
	if sz < n {
		sz = n
	}
	b.data[last] = b.data[last][:b.off]
	b.sz += len(b.data[last])
	b.data = append(b.data, make([]byte, sz))
	b.off = 0
}

// Slice returns a slice of length n to be used for writing.
func (b *BytesBuffer) Slice(n int) []byte {
	b.grow(n)
	last := len(b.data) - 1
	b.off += n
	b.sz += n
	return b.data[last][b.off-n : b.off]
}

// Length returns the size of the buffer.
func (b *BytesBuffer) Length() int {
	return b.sz
}

// CopyTo copies the contents of the buffer to the given byte slice.
// Caller should ensure that o is of appropriate length.
func (b *BytesBuffer) CopyTo(o []byte) int {
	offset := 0
	for i, d := range b.data {
		if i == len(b.data)-1 {
			copy(o[offset:], d[:b.off])
			offset += b.off
		} else {
			copy(o[offset:], d)
			offset += len(d)
		}
	}
	return offset
}

// TruncateBy reduces the size of the bugger by the given amount.
// Always give back <= touched bytes.
func (b *BytesBuffer) TruncateBy(n int) {
	b.off -= n
	b.sz -= n
	AssertTrue(b.off >= 0 && b.sz >= 0)
}

type record struct {
	Name string
	Dur  time.Duration
}

// Timer implements a timer that supports recording the duration of events.
type Timer struct {
	start   time.Time
	last    time.Time
	records []record
}

// Start starts the timer and clears the list of records.
func (t *Timer) Start() {
	t.start = time.Now()
	t.last = t.start
	t.records = t.records[:0]
}

// Record records an event and assigns it the given name.
func (t *Timer) Record(name string) {
	now := time.Now()
	t.records = append(t.records, record{
		Name: name,
		Dur:  now.Sub(t.last).Round(time.Millisecond),
	})
	t.last = now
}

// Total returns the duration since the timer was started.
func (t *Timer) Total() time.Duration {
	return time.Since(t.start).Round(time.Millisecond)
}

func (t *Timer) String() string {
	sort.Slice(t.records, func(i, j int) bool {
		return t.records[i].Dur > t.records[j].Dur
	})
	return fmt.Sprintf("Timer Total: %s. Breakdown: %v", t.Total(), t.records)
}

// PredicateLang extracts the language from a predicate (or facet) name.
// Returns the predicate and the language tag, if any.
func PredicateLang(s string) (string, string) {
	i := strings.LastIndex(s, "@")
	if i <= 0 {
		return s, ""
	}
	return s[0:i], s[i+1:]
}

// DivideAndRule is used to divide a number of tasks among multiple go routines.
func DivideAndRule(num int) (numGo, width int) {
	numGo, width = 64, 0
	for ; numGo >= 1; numGo /= 2 {
		widthF := math.Ceil(float64(num) / float64(numGo))
		if numGo == 1 || widthF >= 256.0 {
			width = int(widthF)
			return
		}
	}
	return
}

// SetupConnection starts a secure gRPC connection to the given host.
func SetupConnection(host string, tlsCfg *tls.Config, useGz bool) (*grpc.ClientConn, error) {
	callOpts := append([]grpc.CallOption{},
		grpc.MaxCallRecvMsgSize(GrpcMaxSize),
		grpc.MaxCallSendMsgSize(GrpcMaxSize))

	if useGz {
		fmt.Fprintf(os.Stderr, "Using compression with %s\n", host)
		callOpts = append(callOpts, grpc.UseCompressor(gzip.Name))
	}

	dialOpts := append([]grpc.DialOption{},
		grpc.WithStatsHandler(&ocgrpc.ClientHandler{}),
		grpc.WithDefaultCallOptions(callOpts...),
		grpc.WithBlock())

	if tlsCfg != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return grpc.DialContext(ctx, host, dialOpts...)
}

// Diff computes the difference between the keys of the two given maps.
func Diff(dst map[string]struct{}, src map[string]struct{}) ([]string, []string) {
	var add []string
	var del []string

	for g := range dst {
		if _, ok := src[g]; !ok {
			add = append(add, g)
		}
	}
	for g := range src {
		if _, ok := dst[g]; !ok {
			del = append(del, g)
		}
	}

	return add, del
}

// SpanTimer returns a function used to record the duration of the given span.
func SpanTimer(span *trace.Span, name string) func() {
	if span == nil {
		return func() {}
	}
	uniq := int64(rand.Int31())
	attrs := []trace.Attribute{
		trace.Int64Attribute("funcId", uniq),
		trace.StringAttribute("funcName", name),
	}
	span.Annotate(attrs, "Start.")
	start := time.Now()

	return func() {
		span.Annotatef(attrs, "End. Took %s", time.Since(start))
		// TODO: We can look into doing a latency record here.
	}
}

// CloseFunc needs to be called to close all the client connections.
type CloseFunc func()

// CredOpt stores the options for logging in, including the password and user.
type CredOpt struct {
	Conf        *viper.Viper
	UserID      string
	PasswordOpt string
}

// GetDgraphClient creates a Dgraph client based on the following options in the configuration:
// --alpha specifies a comma separated list of endpoints to connect to
// --tls_cacert, --tls_cert, --tls_key etc specify the TLS configuration of the connection
// --retries specifies how many times we should retry the connection to each endpoint upon failures
// --user and --password specify the credentials we should use to login with the server
func GetDgraphClient(conf *viper.Viper, login bool) (*dgo.Dgraph, CloseFunc) {
	alphas := conf.GetString("alpha")
	if len(alphas) == 0 {
		glog.Fatalf("The --alpha option must be set in order to connect to Dgraph")
	}

	fmt.Printf("\nRunning transaction with dgraph endpoint: %v\n", alphas)
	tlsCfg, err := LoadClientTLSConfig(conf)
	Checkf(err, "While loading TLS configuration")

	ds := strings.Split(alphas, ",")
	var conns []*grpc.ClientConn
	var clients []api.DgraphClient

	retries := 1
	if conf.IsSet("retries") {
		retries = conf.GetInt("retries")
		if retries < 1 {
			retries = 1
		}
	}

	for _, d := range ds {
		var conn *grpc.ClientConn
		for i := 0; i < retries; i++ {
			conn, err = SetupConnection(d, tlsCfg, false)
			if err == nil {
				break
			}
			fmt.Printf("While trying to setup connection: %v. Retrying...\n", err)
			time.Sleep(time.Second)
		}
		if conn == nil {
			Fatalf("Could not setup connection after %d retries", retries)
		}

		conns = append(conns, conn)
		dc := api.NewDgraphClient(conn)
		clients = append(clients, dc)
	}

	dg := dgo.NewDgraphClient(clients...)
	user := conf.GetString("user")
	if login && len(user) > 0 {
		err = GetPassAndLogin(dg, &CredOpt{
			Conf:        conf,
			UserID:      user,
			PasswordOpt: "password",
		})
		Checkf(err, "While retrieving password and logging in")
	}

	closeFunc := func() {
		for _, c := range conns {
			if err := c.Close(); err != nil {
				glog.Warningf("Error closing connection to Dgraph client: %v", err)
			}
		}
	}
	return dg, closeFunc
}

// AskUserPassword prompts the user to enter the password for the given user ID.
func AskUserPassword(userid string, pwdType string, times int) (string, error) {
	AssertTrue(times == 1 || times == 2)
	AssertTrue(pwdType == "Current" || pwdType == "New")
	// ask for the user's password
	fmt.Printf("%s password for %v:", pwdType, userid)
	pd, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", errors.Wrapf(err, "while reading password")
	}
	fmt.Println()
	password := string(pd)

	if times == 2 {
		fmt.Printf("Retype %s password for %v:", strings.ToLower(pwdType), userid)
		pd2, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return "", errors.Wrapf(err, "while reading password")
		}
		fmt.Println()

		password2 := string(pd2)
		if password2 != password {
			return "", errors.Errorf("the two typed passwords do not match")
		}
	}
	return password, nil
}

// GetPassAndLogin uses the given credentials and client to perform the login operation.
func GetPassAndLogin(dg *dgo.Dgraph, opt *CredOpt) error {
	password := opt.Conf.GetString(opt.PasswordOpt)
	if len(password) == 0 {
		var err error
		password, err = AskUserPassword(opt.UserID, "Current", 1)
		if err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := dg.Login(ctx, opt.UserID, password); err != nil {
		return errors.Wrapf(err, "unable to login to the %v account", opt.UserID)
	}
	fmt.Println("Login successful.")
	// update the context so that it has the admin jwt token
	return nil
}
