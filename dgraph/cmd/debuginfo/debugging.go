/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package debuginfo

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
)

func saveMetrics(addr, pathPrefix string, seconds uint32, metricTypes []string) {
	u, err := url.Parse(addr)
	if err != nil || (u.Host == "" && u.Scheme != "" && u.Scheme != "file") {
		u, err = url.Parse("http://" + addr)
	}
	if err != nil || u.Host == "" {
		glog.Errorf("error while parsing address %s: %s", addr, err)
		return
	}

	duration := time.Duration(seconds) * time.Second

	for _, metricType := range metricTypes {
		source := u.String() + metricMap[metricType]
		switch metricType {
		case "cpu":
			source += fmt.Sprintf("%s%d", "?seconds=", seconds)
		case "trace":
			source += fmt.Sprintf("%s%d", "?seconds=", seconds)
		}
		savePath := fmt.Sprintf("%s%s.gz", pathPrefix, metricType)
		if err := saveDebug(source, savePath, duration); err != nil {
			glog.Errorf("error while saving metric from %s: %s", source, err)
			continue
		}

		glog.Infof("saving %s metric in %s", metricType, savePath)
	}
}

// saveDebug writes the debug info specified in the argument fetching it from the host
// provided in the configuration
func saveDebug(sourceURL, filePath string, duration time.Duration) error {
	var err error
	var resp io.ReadCloser

	glog.Infof("fetching information over HTTP from %s", sourceURL)
	if duration > 0 {
		glog.Info(fmt.Sprintf("please wait... (%v)", duration))
	}

	timeout := duration + duration/2 + 2*time.Second
	resp, err = fetchURL(sourceURL, timeout)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Close(); err != nil {
			glog.Warningf("error closing resp reader: %v", err)
		}
	}()
	out, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error while creating debug file: %s", err)
	}
	defer func() {
		out.Close()
	}()
	_, err = io.Copy(out, resp)
	return err
}

// fetchURL fetches a profile from a URL using HTTP.
func fetchURL(source string, timeout time.Duration) (io.ReadCloser, error) {
	client := &http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(source)
	if err != nil {
		return nil, fmt.Errorf("http fetch: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				glog.Warningf("error closing body: %v", err)
			}
		}()
		return nil, statusCodeError(resp)
	}

	return resp.Body, nil
}

func statusCodeError(resp *http.Response) error {
	if resp.Header.Get("X-Go-Pprof") != "" &&
		strings.Contains(resp.Header.Get("Content-Type"), "text/plain") {
		if body, err := io.ReadAll(resp.Body); err == nil {
			return fmt.Errorf("server response: %s - %s", resp.Status, body)
		}
	}
	return fmt.Errorf("server response: %s", resp.Status)
}
