// +build oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package enc

import (
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"io"
)

// Eebuild indicates if this is a Enterprise build.
var EeBuild = false

// GetWriter returns the Writer as is for OSS Builds.
func GetWriter(_ []byte, w io.Writer) (io.Writer, error) {
	return w, nil
}

// GetReader returns the reader as is for OSS Builds.
func GetReader(_ []byte, r io.Reader) (io.Reader, error) {
	return r, nil
}

// RegisterVaultFlags registers the required encryption flags. None for OSS.
func RegisterFlags(_ *pflag.FlagSet) {
	return
}

// ReadKey reads the key. Nil for OSS.
func ReadKey(_ *viper.Viper) (x.SensitiveByteSlice, error) {
	return nil, nil
}
