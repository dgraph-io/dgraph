/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

// session holds the session config for a handler.
type session struct {
	host, path, file string
	args             map[string][]string
	size             int64
}

func (s *session) Getarg(key string) string {
	if v, ok := s.args[key]; ok && len(v) > 0 {
		return v[0]
	}
	return ""
}
