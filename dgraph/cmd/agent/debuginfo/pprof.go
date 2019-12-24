/*
 * Copyright 2019-2020 Dgraph Labs, Inc. and Contributors
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

package debuginfo

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
)

type pprofCollector struct {
	baseDir  string
	duration time.Duration
	timeout  time.Duration
	tr       http.RoundTripper

	profileTypes []string
}

var pprofProfileTypes = []string{
	"goroutine",
	"heap",
	"threadcreate",
	"block",
	"mutex",
	"profile",
	"trace",
}

func newPprofCollector(baseDir string, duration time.Duration,
	profileTypes []string) *pprofCollector {
	timeout := duration + duration/2

	var transport http.RoundTripper = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: timeout,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &pprofCollector{
		baseDir:  baseDir,
		duration: duration,
		timeout:  timeout,
		tr:       transport,

		profileTypes: profileTypes,
	}
}

// Collect all the profiles and save them to the directory specified in baseDir.
func (c *pprofCollector) Collect(host, filePrefix string) {
	for _, pType := range c.profileTypes {
		src, err := c.saveProfile(host, pType, filePrefix)
		if err != nil {
			glog.Errorf("error while saving pprof profile from %s: %s", src, err)
			continue
		}

		glog.Infof("saving %s profile in %s", pType,
			filepath.Join(filepath.Base(c.baseDir), fmt.Sprintf("%s%s.gz", filePrefix, pType)))
	}
}

// saveProfile writes the profile specified in the argument fetching it from the host
// provided in the configuration
func (c *pprofCollector) saveProfile(host, profileType, filePrefix string) (src string, err error) {
	var resp io.ReadCloser
	source := fmt.Sprintf("%s/debug/pprof/%s", host, profileType)

	if sourceURL, timeout := adjustURL(source, c.duration, c.timeout); sourceURL != "" {
		glog.Info("fetching profile over HTTP from " + sourceURL)
		if c.duration > 0 {
			glog.Info(fmt.Sprintf("please wait... (%v)", c.duration))
		}
		resp, err = fetchURL(sourceURL, timeout, c.tr)
		src = sourceURL
	}
	if err != nil {
		return
	}

	defer resp.Close()
	out, err := os.Create(filepath.Join(c.baseDir, fmt.Sprintf("%s%s.gz", filePrefix, profileType)))
	if err != nil {
		return
	}
	_, err = io.Copy(out, resp)
	return
}

// fetchURL fetches a profile from a URL using HTTP.
func fetchURL(source string, timeout time.Duration, tr http.RoundTripper) (io.ReadCloser, error) {
	client := &http.Client{
		Transport: tr,
		Timeout:   timeout + 5*time.Second,
	}
	resp, err := client.Get(source)
	if err != nil {
		return nil, fmt.Errorf("http fetch: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, statusCodeError(resp)
	}

	return resp.Body, nil
}

func statusCodeError(resp *http.Response) error {
	if resp.Header.Get("X-Go-Pprof") != "" &&
		strings.Contains(resp.Header.Get("Content-Type"), "text/plain") {
		// error is from pprof endpoint
		if body, err := ioutil.ReadAll(resp.Body); err == nil {
			return fmt.Errorf("server response: %s - %s", resp.Status, body)
		}
	}
	return fmt.Errorf("server response: %s", resp.Status)
}

// adjustURL validates if a profile source is a URL and returns an
// cleaned up URL and the timeout to use for retrieval over HTTP.
// If the source cannot be recognized as a URL it returns an empty string.
func adjustURL(source string, duration, timeout time.Duration) (string, time.Duration) {
	u, err := url.Parse(source)
	if err != nil || (u.Host == "" && u.Scheme != "" && u.Scheme != "file") {
		// Try adding http:// to catch sources of the form hostname:port/path.
		// url.Parse treats "hostname" as the scheme.
		u, err = url.Parse("http://" + source)
	}
	if err != nil || u.Host == "" {
		return "", 0
	}

	// Apply duration/timeout overrides to URL.
	values := u.Query()
	if duration > 0 {
		values.Set("seconds", fmt.Sprint(int(duration.Seconds())))
	} else {
		if urlSeconds := values.Get("seconds"); urlSeconds != "" {
			if us, err := strconv.ParseInt(urlSeconds, 10, 32); err == nil {
				duration = time.Duration(us) * time.Second
			}
		}
	}
	if timeout <= 0 {
		if duration > 0 {
			timeout = duration + duration/2
		} else {
			timeout = 60 * time.Second
		}
	}
	u.RawQuery = values.Encode()
	return u.String(), timeout
}
