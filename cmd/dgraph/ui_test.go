/*
 * Copyright 2017 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func populateAssets(t *testing.T) string {
	indexContent := []byte(`<head>
    <script>
      window.SERVER_PORT = __SERVER_PORT__;
    </script>
</head>`)

	dir, err := ioutil.TempDir("", "example")
	require.NoError(t, err)

	index := filepath.Join(dir, "index.html")
	err = ioutil.WriteFile(index, indexContent, 0666)
	require.NoError(t, err)

	os.MkdirAll(dir+"/static/css", 0755)
	cssContent := []byte(`
body {
	width: 100%;
}`)
	css := filepath.Join(dir, "/static/css/custom.css")
	err = ioutil.WriteFile(css, cssContent, 0666)
	require.NoError(t, err)

	return dir
}

func TestSubstitutePort(t *testing.T) {
	dir := populateAssets(t)
	defer os.RemoveAll(dir)

	*uiDir = dir
	*port = 8082
	indexHtml := substitutePort()
	require.Equal(t, `<head>
    <script>
      window.SERVER_PORT = 8082;
    </script>
</head>`, indexHtml)
}

func TestReplacePort(t *testing.T) {
	dir := populateAssets(t)
	defer os.RemoveAll(dir)

	*uiDir = dir
	*port = 8082
	indexHtml := substitutePort()

	// Lets hit the root route and see that we get back index.html with port substituted.
	req, err := http.NewRequest("GET", "/", nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	handler := replacePort(http.FileServer(http.Dir(*uiDir)), indexHtml)
	handler.ServeHTTP(rr, req)
	require.Equal(t, `<head>
    <script>
      window.SERVER_PORT = 8082;
    </script>
</head>`, rr.Body.String())

	// Lets try to get back some CSS from the uiDir.
	req, err = http.NewRequest("GET", "/static/css/custom.css", nil)
	require.NoError(t, err)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, `
body {
	width: 100%;
}`, rr.Body.String())
}

func TestReplacePortNoDir(t *testing.T) {
	*uiDir = "/tmp/random"
	indexHtml := substitutePort()

	// Lets hit the root route and see that we get back index.html with port substituted.
	req, err := http.NewRequest("GET", "/", nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	handler := replacePort(http.FileServer(http.Dir(*uiDir)), indexHtml)
	handler.ServeHTTP(rr, req)
	require.Equal(t, "Page not found", rr.Body.String())
}
