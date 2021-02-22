/*
 * Copyright 2015-2021 Dgraph Labs, Inc. and Contributors
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
	builtinGzip "compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc/peer"

	"github.com/dgraph-io/badger/v3"
	bo "github.com/dgraph-io/badger/v3/options"
	"github.com/dgraph-io/badger/v3/pb"
	badgerpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/ristretto/z"
	"github.com/dustin/go-humanize"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/trace"
	"golang.org/x/crypto/ssh/terminal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Error constants representing different types of errors.
var (
	// ErrNotSupported is thrown when an enterprise feature is requested in the open source version.
	ErrNotSupported = errors.Errorf("Feature available only in Dgraph Enterprise Edition")
	// ErrNoJwt is returned when JWT is not present in the context.
	ErrNoJwt = errors.New("no accessJwt available")
	// ErrorInvalidLogin is returned when username or password is incorrect in login
	ErrorInvalidLogin = errors.New("invalid username or password")
	// ErrConflict is returned when commit couldn't succeed due to conflicts.
	ErrConflict = errors.New("Transaction conflict")
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
	ValidHostnameRegex = `^([a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62}){1}(\.[a-zA-Z0-9_]{1}` +
		`[a-zA-Z0-9_-]{0,62})*[._]?$`
	// Star is equivalent to using * in a mutation.
	// When changing this value also remember to change in in client/client.go:DeleteEdges.
	Star = "_STAR_ALL"

	// GrpcMaxSize is the maximum possible size for a gRPC message.
	// Dgraph uses the maximum size for the most flexibility (2GB - equal
	// to the max grpc frame size). Users will still need to set the max
	// message sizes allowable on the client size when dialing.
	GrpcMaxSize = math.MaxInt32

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
	// GuardiansId is the ID of the admin group for ACLs.
	GuardiansId = "guardians"

	// GroupIdFileName is the name of the file storing the ID of the group to which
	// the data in a postings directory belongs. This ID is used to join the proper
	// group the first time an Alpha comes up with data from a restored backup or a
	// bulk load.
	GroupIdFileName = "group_id"

	// DefaultCreds is the default credentials for login via dgo client.
	DefaultCreds = "user=; password=; namespace=0;"

	AccessControlAllowedHeaders = "X-Dgraph-AccessToken, X-Dgraph-AuthToken, " +
		"Content-Type, Content-Length, Accept-Encoding, Cache-Control, " +
		"X-CSRF-Token, X-Auth-Token, X-Requested-With"
	DgraphCostHeader = "Dgraph-TouchedUids"

	DgraphVersion = 2103
)

var (
	// Useful for running multiple servers on the same machine.
	regExpHostName = regexp.MustCompile(ValidHostnameRegex)
	// Nilbyte is a nil byte slice. Used
	Nilbyte []byte
	// GuardiansUid is a map from namespace to the Uid of guardians group node.
	GuardiansUid = &sync.Map{}
	// GrootUser Uid is a map from namespace to the Uid of groot user node.
	GrootUid = &sync.Map{}
)

func init() {
	GuardiansUid.Store(GalaxyNamespace, 0)
	GrootUid.Store(GalaxyNamespace, 0)

}

// ShouldCrash returns true if the error should cause the process to crash.
func ShouldCrash(err error) bool {
	if err == nil {
		return false
	}
	errStr := status.Convert(err).Message()
	return strings.Contains(errStr, "REUSE_RAFTID") ||
		strings.Contains(errStr, "REUSE_ADDR") ||
		strings.Contains(errStr, "NO_ADDR") ||
		strings.Contains(errStr, "ENTERPRISE_LIMIT_REACHED") ||
		strings.Contains(errStr, "ENTERPRISE_ONLY_LEARNER")
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

// GqlErrorList is a list of GraphQL errors as would be found in a response.
type GqlErrorList []*GqlError

type queryRes struct {
	Errors GqlErrorList `json:"errors"`
}

// IsGqlErrorList tells whether the given err is a list of GraphQL errors.
func IsGqlErrorList(err error) bool {
	if _, ok := err.(GqlErrorList); ok {
		return true
	}
	return false
}

func (gqlErr *GqlError) Error() string {
	var buf bytes.Buffer
	if gqlErr == nil {
		return ""
	}

	Check2(buf.WriteString(gqlErr.Message))

	if len(gqlErr.Locations) > 0 {
		Check2(buf.WriteString(" (Locations: ["))
		for i, loc := range gqlErr.Locations {
			if i > 0 {
				Check2(buf.WriteString(", "))
			}
			Check2(buf.WriteString(fmt.Sprintf("{Line: %v, Column: %v}", loc.Line, loc.Column)))
		}
		Check2(buf.WriteString("])"))
	}

	return buf.String()
}

func (errList GqlErrorList) Error() string {
	var buf bytes.Buffer
	for i, gqlErr := range errList {
		if i > 0 {
			Check(buf.WriteByte('\n'))
		}
		Check2(buf.WriteString(gqlErr.Error()))
	}
	return buf.String()
}

// GqlErrorf returns a new GqlError with the message and args Sprintf'ed as the
// GqlError's Message.
func GqlErrorf(message string, args ...interface{}) *GqlError {
	return &GqlError{
		Message: fmt.Sprintf(message, args...),
	}
}

// ExtractNamespaceHTTP parses the namespace value from the incoming HTTP request.
func ExtractNamespaceHTTP(r *http.Request) uint64 {
	ctx := AttachAccessJwt(context.Background(), r)
	// Ignoring error because the default value is zero anyways.
	namespace, _ := ExtractJWTNamespace(ctx)
	return namespace
}

// ExtractNamespace parses the namespace value from the incoming gRPC context. For the non-ACL mode,
// it is caller's responsibility to set the galaxy namespace.
func ExtractNamespace(ctx context.Context) (uint64, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0, errors.New("No metadata in the context")
	}
	ns := md.Get("namespace")
	if len(ns) == 0 {
		return 0, errors.New("No namespace in the metadata of context")
	}
	namespace, err := strconv.ParseUint(ns[0], 0, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "Error while parsing namespace from metadata")
	}
	return namespace, nil
}

func IsGalaxyOperation(ctx context.Context) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	ns := md.Get("galaxy-operation")
	return len(ns) > 0 && (ns[0] == "true" || ns[0] == "True")
}

func GetForceNamespace(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	ns := md.Get("force-namespace")
	if len(ns) == 0 {
		return ""
	}
	return ns[0]
}

func ExtractJwt(ctx context.Context) ([]string, error) {
	// extract the jwt and unmarshal the jwt to get the list of groups
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, ErrNoJwt
	}
	accessJwt := md.Get("accessJwt")
	if len(accessJwt) == 0 {
		return nil, ErrNoJwt
	}

	return accessJwt, nil
}

// WithLocations adds a list of locations to a GqlError and returns the same
// GqlError (fluent style).
func (gqlErr *GqlError) WithLocations(locs ...Location) *GqlError {
	if gqlErr == nil {
		return nil
	}

	gqlErr.Locations = append(gqlErr.Locations, locs...)
	return gqlErr
}

// WithPath adds a path to a GqlError and returns the same
// GqlError (fluent style).
func (gqlErr *GqlError) WithPath(path []interface{}) *GqlError {
	if gqlErr == nil {
		return nil
	}

	gqlErr.Path = path
	return gqlErr
}

// SetStatus sets the error code, message and the newly assigned uids
// in the http response.
func SetStatus(w http.ResponseWriter, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	var qr queryRes
	ext := make(map[string]interface{})
	ext["code"] = code
	qr.Errors = append(qr.Errors, &GqlError{Message: msg, Extensions: ext})
	if js, err := json.Marshal(qr); err == nil {
		if _, err := w.Write(js); err != nil {
			glog.Errorf("Error while writing: %+v", err)
		}
	} else {
		Panic(errors.Errorf("Unable to marshal: %+v", qr))
	}
}

func SetStatusWithErrors(w http.ResponseWriter, code string, errs []string) {
	var qr queryRes
	ext := make(map[string]interface{})
	ext["code"] = code
	for _, err := range errs {
		qr.Errors = append(qr.Errors, &GqlError{Message: err, Extensions: ext})
	}
	if js, err := json.Marshal(qr); err == nil {
		if _, err := w.Write(js); err != nil {
			glog.Errorf("Error while writing: %+v", err)
		}
	} else {
		Panic(errors.Errorf("Unable to marshal: %+v", qr))
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
	w.Header().Set("Access-Control-Allow-Headers", AccessControlAllowedHeaders)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Connection", "close")
}

// QueryResWithData represents a response that holds errors as well as data.
type QueryResWithData struct {
	Errors GqlErrorList `json:"errors"`
	Data   *string      `json:"data"`
}

// SetStatusWithData sets the errors in the response and ensures that the data key
// in the data is present with value nil.
// In case an error was encountered after the query execution started, we have to return data
// key with null value according to GraphQL spec.
func SetStatusWithData(w http.ResponseWriter, code, msg string) {
	var qr QueryResWithData
	ext := make(map[string]interface{})
	ext["code"] = code
	qr.Errors = append(qr.Errors, &GqlError{Message: msg, Extensions: ext})
	// This would ensure that data key is present with value null.
	if js, err := json.Marshal(qr); err == nil {
		if _, err := w.Write(js); err != nil {
			glog.Errorf("Error while writing: %+v", err)
		}
	} else {
		Panic(errors.Errorf("Unable to marshal: %+v", qr))
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

// AttachJWTNamespace attaches the namespace in the JWT claims to the context if present, otherwise
// it attaches the galaxy namespace.
func AttachJWTNamespace(ctx context.Context) context.Context {
	if WorkerConfig.AclEnabled {
		ns, err := ExtractJWTNamespace(ctx)
		if err != nil {
			glog.Errorf("Failed to get namespace from the accessJWT token: Error: %s", err)
		} else {
			// Attach the namespace only if we got one from JWT.
			// This preserves any namespace directly present in the context which is needed for
			// requests originating from dgraph internal code like server.go::GetGQLSchema() where
			// context is created by hand.
			ctx = AttachNamespace(ctx, ns)
		}
	} else {
		ctx = AttachNamespace(ctx, GalaxyNamespace)
	}
	return ctx
}

// AttachNamespace adds given namespace to the metadata of the context.
func AttachNamespace(ctx context.Context, namespace uint64) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}
	ns := strconv.FormatUint(namespace, 10)
	md.Set("namespace", ns)
	return metadata.NewIncomingContext(ctx, md)
}

// AttachGalaxyOperation specifies in the context that it will be used for doing a galaxy operation.
func AttachGalaxyOperation(ctx context.Context, ns uint64) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}
	md.Set("galaxy-operation", "true")
	md.Set("force-namespace", strconv.FormatUint(ns, 10))
	return metadata.NewOutgoingContext(ctx, md)
}

// AttachAuthToken adds any incoming PoorMan's auth header data into the grpc context metadata
func AttachAuthToken(ctx context.Context, r *http.Request) context.Context {
	if authToken := r.Header.Get("X-Dgraph-AuthToken"); authToken != "" {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}

		md.Append("auth-token", authToken)
		ctx = metadata.NewIncomingContext(ctx, md)
	}
	return ctx
}

// AttachAccessJwt adds any incoming JWT header data into the grpc context metadata
func AttachAccessJwt(ctx context.Context, r *http.Request) context.Context {
	if accessJwt := r.Header.Get("X-Dgraph-AccessToken"); accessJwt != "" {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}

		md.Append("accessJwt", accessJwt)
		ctx = metadata.NewIncomingContext(ctx, md)
	}
	return ctx
}

// AttachRemoteIP adds any incoming IP data into the grpc context metadata
func AttachRemoteIP(ctx context.Context, r *http.Request) context.Context {
	if ip, port, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if intPort, convErr := strconv.Atoi(port); convErr == nil {
			ctx = peer.NewContext(ctx, &peer.Peer{
				Addr: &net.TCPAddr{
					IP:   net.ParseIP(ip),
					Port: intPort,
				},
			})
		}
	}
	return ctx
}

// isIpWhitelisted checks if the given ipString is within the whitelisted ip range
func isIpWhitelisted(ipString string) bool {
	ip := net.ParseIP(ipString)

	if ip == nil {
		return false
	}

	if ip.IsLoopback() {
		return true
	}

	for _, ipRange := range WorkerConfig.WhiteListedIPRanges {
		if bytes.Compare(ip, ipRange.Lower) >= 0 && bytes.Compare(ip, ipRange.Upper) <= 0 {
			return true
		}
	}
	return false
}

// HasWhitelistedIP checks whether the source IP in ctx is whitelisted or not.
// It returns the IP address if the IP is whitelisted, otherwise an error is returned.
func HasWhitelistedIP(ctx context.Context) (net.Addr, error) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		return nil, errors.New("unable to find source ip")
	}
	ip, _, err := net.SplitHostPort(peerInfo.Addr.String())
	if err != nil {
		return nil, err
	}
	if !isIpWhitelisted(ip) {
		return nil, errors.Errorf("unauthorized ip address: %s", ip)
	}
	return peerInfo.Addr, nil
}

// Write response body, transparently compressing if necessary.
func WriteResponse(w http.ResponseWriter, r *http.Request, b []byte) (int, error) {
	var out io.Writer = w

	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gzw := builtinGzip.NewWriter(w)
		defer gzw.Close()
		out = gzw
	}

	bytesWritten, err := out.Write(b)
	if err != nil {
		return 0, err
	}
	w.Header().Set("Content-Length", strconv.FormatInt(int64(bytesWritten), 10))
	return bytesWritten, nil
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

// Unique takes an array and returns it with no duplicate entries.
func Unique(a []string) []string {
	if len(a) < 2 {
		return a
	}

	sort.Strings(a)
	idx := 1
	for _, val := range a {
		if a[idx-1] == val {
			continue
		}
		a[idx] = val
		idx++
	}
	return a[:idx]
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
func ValidateAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if p, err := strconv.Atoi(port); err != nil || p <= 0 || p >= 65536 {
		return errors.Errorf("Invalid port: %v", p)
	}
	if ip := net.ParseIP(host); ip != nil {
		return nil
	}
	// try to parse as hostname as per hostname RFC
	if len(strings.Replace(host, ".", "", -1)) > 255 {
		return errors.Errorf("Hostname should be less than or equal to 255 characters")
	}
	if !regExpHostName.MatchString(host) {
		return errors.Errorf("Invalid hostname: %v", host)
	}
	return nil
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
func SetupConnection(host string, tlsCfg *tls.Config, useGz bool, dialOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	callOpts := append([]grpc.CallOption{},
		grpc.MaxCallRecvMsgSize(GrpcMaxSize),
		grpc.MaxCallSendMsgSize(GrpcMaxSize))

	if useGz {
		fmt.Fprintf(os.Stderr, "Using compression with %s\n", host)
		callOpts = append(callOpts, grpc.UseCompressor(gzip.Name))
	}

	dialOpts = append(dialOpts,
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
	UserID    string
	Password  string
	Namespace uint64
}

type authorizationCredentials struct {
	token string
}

func (a *authorizationCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{"Authorization": a.token}, nil
}

func (a *authorizationCredentials) RequireTransportSecurity() bool {
	return true
}

// WithAuthorizationCredentials adds Authorization: <api-token> to every GRPC request
// This is mostly used by Slash GraphQL to authenticate requests
func WithAuthorizationCredentials(authToken string) grpc.DialOption {
	return grpc.WithPerRPCCredentials(&authorizationCredentials{authToken})
}

// GetDgraphClient creates a Dgraph client based on the following options in the configuration:
// --slash_grpc_endpoint specifies the grpc endpoint for slash. It takes precedence over --alpha and TLS
// --alpha specifies a comma separated list of endpoints to connect to
// --tls_cacert, --tls_cert, --tls_key etc specify the TLS configuration of the connection
// --retries specifies how many times we should retry the connection to each endpoint upon failures
// --user and --password specify the credentials we should use to login with the server
func GetDgraphClient(conf *viper.Viper, login bool) (*dgo.Dgraph, CloseFunc) {
	var alphas string
	if conf.GetString("slash_grpc_endpoint") != "" {
		alphas = conf.GetString("slash_grpc_endpoint")
	} else {
		alphas = conf.GetString("alpha")
	}

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

	dialOpts := []grpc.DialOption{}
	if conf.GetString("slash_grpc_endpoint") != "" && conf.IsSet("auth_token") {
		dialOpts = append(dialOpts, WithAuthorizationCredentials(conf.GetString("auth_token")))
	}

	for _, d := range ds {
		var conn *grpc.ClientConn
		for i := 0; i < retries; i++ {
			conn, err = SetupConnection(d, tlsCfg, false, dialOpts...)
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
	creds := z.NewSuperFlag(conf.GetString("creds"))
	user := creds.GetString("user")
	if login && len(user) > 0 {
		err = GetPassAndLogin(dg, &CredOpt{
			UserID:    user,
			Password:  creds.GetString("password"),
			Namespace: creds.GetUint64("namespace"),
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
	password := opt.Password
	if len(password) == 0 {
		var err error
		password, err = AskUserPassword(opt.UserID, "Current", 1)
		if err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := dg.LoginIntoNamespace(ctx, opt.UserID, password, opt.Namespace); err != nil {
		return errors.Wrapf(err, "unable to login to the %v account", opt.UserID)
	}
	fmt.Println("Login successful.")
	// update the context so that it has the admin jwt token
	return nil
}

func IsGuardian(groups []string) bool {
	for _, group := range groups {
		if group == GuardiansId {
			return true
		}
	}

	return false
}

// RunVlogGC runs value log gc on store. It runs GC unconditionally after every 10 minutes.
// Additionally it also runs GC if vLogSize has grown more than 1 GB in last minute.
func RunVlogGC(store *badger.DB, closer *z.Closer) {
	defer closer.Done()

	// Runs every 1m, checks size of vlog and runs GC conditionally.
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	abs := func(a, b int64) int64 {
		if a > b {
			return a - b
		}
		return b - a
	}

	var lastSz int64
	runGC := func() {
		for err := error(nil); err == nil; {
			// If a GC is successful, immediately run it again.
			err = store.RunValueLogGC(0.7)
		}
		_, sz := store.Size()
		if abs(lastSz, sz) > 512<<20 {
			glog.V(2).Infof("Value log size: %s\n", humanize.IBytes(uint64(sz)))
			lastSz = sz
		}
	}

	runGC()
	for {
		select {
		case <-closer.HasBeenClosed():
			return
		case <-ticker.C:
			runGC()
		}
	}
}

type DB interface {
	Sync() error
}

func StoreSync(db DB, closer *z.Closer) {
	defer closer.Done()
	// We technically don't need to call this due to mmap being able to survive process crashes.
	// But, once a minute is infrequent enough that we won't lose any performance due to this.
	ticker := time.NewTicker(time.Minute)
	for {
		select {
		case <-ticker.C:
			if err := db.Sync(); err != nil {
				glog.Errorf("Error while calling db sync: %+v", err)
			}
		case <-closer.HasBeenClosed():
			return
		}
	}
}

// DeepCopyJsonMap returns a deep copy of the input map `m`.
// `m` is supposed to be a map similar to the ones produced as a result of json unmarshalling. i.e.,
// any value in `m` at any nested level should be of an inbuilt go type.
func DeepCopyJsonMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}

	mCopy := make(map[string]interface{})
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			mCopy[k] = DeepCopyJsonMap(val)
		case []interface{}:
			mCopy[k] = DeepCopyJsonArray(val)
		default:
			mCopy[k] = val
		}
	}
	return mCopy
}

// DeepCopyJsonArray returns a deep copy of the input array `a`.
// `a` is supposed to be an array similar to the ones produced as a result of json unmarshalling.
// i.e., any value in `a` at any nested level should be of an inbuilt go type.
func DeepCopyJsonArray(a []interface{}) []interface{} {
	if a == nil {
		return nil
	}

	aCopy := make([]interface{}, 0, len(a))
	for _, v := range a {
		switch val := v.(type) {
		case map[string]interface{}:
			aCopy = append(aCopy, DeepCopyJsonMap(val))
		case []interface{}:
			aCopy = append(aCopy, DeepCopyJsonArray(val))
		default:
			aCopy = append(aCopy, val)
		}
	}
	return aCopy
}

// GetCachePercentages returns the slice of cache percentages given the "," (comma) separated
// cache percentages(integers) string and expected number of caches.
func GetCachePercentages(cpString string, numExpected int) ([]int64, error) {
	cp := strings.Split(cpString, ",")
	// Sanity checks
	if len(cp) != numExpected {
		return nil, errors.Errorf("ERROR: expected %d cache percentages, got %d",
			numExpected, len(cp))
	}

	var cachePercent []int64
	percentSum := 0
	for _, percent := range cp {
		x, err := strconv.Atoi(percent)
		if err != nil {
			return nil, errors.Errorf("ERROR: unable to parse cache percentage(%s)", percent)
		}
		if x < 0 {
			return nil, errors.Errorf("ERROR: cache percentage(%s) cannot be negative", percent)
		}
		cachePercent = append(cachePercent, int64(x))
		percentSum += x
	}

	if percentSum != 100 {
		return nil, errors.Errorf("ERROR: cache percentages (%s) does not sum up to 100",
			strings.Join(cp, "+"))
	}

	return cachePercent, nil
}

// ParseCompression returns badger.compressionType and compression level given compression string
// of format compression-type:compression-level
func ParseCompression(cStr string) (bo.CompressionType, int) {
	cStrSplit := strings.Split(cStr, ":")
	cType := cStrSplit[0]
	level := 3

	var err error
	if len(cStrSplit) == 2 {
		level, err = strconv.Atoi(cStrSplit[1])
		Check(err)
		if level <= 0 {
			glog.Fatalf("ERROR: compression level(%v) must be greater than zero", level)
		}
	} else if len(cStrSplit) > 2 {
		glog.Fatalf("ERROR: Invalid badger.compression argument")
	}
	switch cType {
	case "zstd":
		return bo.ZSTD, level
	case "snappy":
		return bo.Snappy, 0
	case "none":
		return bo.None, 0
	}
	glog.Fatalf("ERROR: compression type (%s) invalid", cType)
	return 0, 0
}

// ToHex converts a uint64 to a hex byte array. If rdf is true it will
// use < > brackets to delimit the value. Otherwise it will use quotes
// like JSON requires.
func ToHex(i uint64, rdf bool) []byte {
	var b [16]byte
	tmp := strconv.AppendUint(b[:0], i, 16)

	out := make([]byte, len(tmp)+3+1)
	if rdf {
		out[0] = '<'
	} else {
		out[0] = '"'
	}

	out[1] = '0'
	out[2] = 'x'
	n := copy(out[3:], tmp)

	if rdf {
		out[3+n] = '>'
	} else {
		out[3+n] = '"'
	}

	return out
}

// RootTemplate defines the help template for dgraph command.
var RootTemplate string = `Dgraph is a horizontally scalable and distributed graph database,
providing ACID transactions, consistent replication and linearizable reads.
It's built from the ground up to perform for a rich set of queries. Being a native
graph database, it tightly controls how the data is arranged on disk to optimize
for query performance and throughput, reducing disk seeks and network calls in a
cluster.` + BuildDetails() +
	`Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}} {{if .HasAvailableSubCommands}}

Generic: {{range .Commands}} {{if (or (and .IsAvailableCommand (eq .Annotations.group "default")) (eq .Name "help"))}}
 {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Available Commands:

Dgraph Core: {{range .Commands}} {{if (and .IsAvailableCommand (eq .Annotations.group "core"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Data Loading: {{range .Commands}} {{if (and .IsAvailableCommand (eq .Annotations.group "data-load"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Dgraph Security: {{range .Commands}} {{if (and .IsAvailableCommand (eq .Annotations.group "security"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Dgraph Debug: {{range .Commands}} {{if (and .IsAvailableCommand (eq .Annotations.group "debug"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Dgraph Tools: {{range .Commands}} {{if (and .IsAvailableCommand (eq .Annotations.group "tool"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}
` +
	// uncomment this part when new availalble commands are added

	/*Additional Commands:{{range .Commands}}{{if (and .IsAvailableCommand (not .Annotations.group))}}
	  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}*/
	`
Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}


Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// NonRootTemplate defines the help template for dgraph sub-command.
var NonRootTemplate string = `{{if .Long}} {{.Long}} {{else}} {{.Short}} {{end}}
Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}} {{if .HasAvailableSubCommands}}

Available Commands: {{range .Commands}}{{if (or .IsAvailableCommand)}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// KvWithMaxVersion returns a KV with the max version from the list of KVs.
func KvWithMaxVersion(kvs *badgerpb.KVList, prefixes [][]byte) *badgerpb.KV {
	// Iterate over kvs to get the KV with the latest version. It is not necessary that the last
	// KV contain the latest value.
	var maxKv *badgerpb.KV
	for _, kv := range kvs.GetKv() {
		if maxKv.GetVersion() <= kv.GetVersion() {
			maxKv = kv
		}
	}
	return maxKv
}

// PrefixesToMatches converts the prefixes for subscription to a list of match.
func PrefixesToMatches(prefixes [][]byte, ignore string) []*pb.Match {
	matches := make([]*pb.Match, 0, len(prefixes))
	for _, prefix := range prefixes {
		matches = append(matches, &pb.Match{
			Prefix:      prefix,
			IgnoreBytes: ignore,
		})
	}
	return matches
}
