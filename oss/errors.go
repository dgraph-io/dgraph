// +build oss

package oss

import "errors"

var ErrNotSupported = errors.New("Feature available only in Dgraph Enterprise Edition.")