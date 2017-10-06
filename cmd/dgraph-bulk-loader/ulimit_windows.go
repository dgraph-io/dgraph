// +build windows

package main

import "github.com/pkg/errors"

func queryMaxOpenFiles() (int, error) {
	return 0, errors.New("cannot detect max open files on this platform")
}
