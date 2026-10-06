/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/pkg/errors"
)

// TabletPlacement pins one predicate to one group. A placement file is a JSON
// array of these entries and is the shared contract between the bulk loader and
// Zero's tablet-pinning APIs: entries carry groups, never shard numbers.
type TabletPlacement struct {
	Predicate string `json:"predicate"`
	Group     uint32 `json:"group"`
	// Namespace is optional in the file and defaults to the root namespace.
	Namespace uint64 `json:"namespace"`
}

func (p TabletPlacement) String() string {
	return fmt.Sprintf("[%d] %s -> group %d", p.Namespace, p.Predicate, p.Group)
}

// ParseTabletPlacement decodes and validates a tablet placement document: a JSON
// array of {predicate, group, namespace} entries. It rejects unknown fields,
// empty predicates, groups below 1, duplicate (namespace, predicate) pairs and
// reserved (dgraph.*) predicates, reporting every offending entry in one error.
// Upper-bounding the group is the caller's job — only the caller knows how many
// groups exist.
func ParseTabletPlacement(r io.Reader) ([]TabletPlacement, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	var entries []TabletPlacement
	if err := dec.Decode(&entries); err != nil {
		return nil, errors.Wrap(err, "parsing tablet placement")
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("parsing tablet placement: trailing data after the JSON array")
	}

	var faults []string
	seen := make(map[TabletPlacement]struct{}, len(entries))
	for i, e := range entries {
		switch {
		case e.Predicate == "":
			faults = append(faults, fmt.Sprintf("entry %d: empty predicate", i))
		case isReservedName(e.Predicate):
			faults = append(faults, fmt.Sprintf("entry %d (%s): reserved predicates are always "+
				"served by group 1 and cannot be placed", i, e))
		}
		if e.Group < 1 {
			faults = append(faults, fmt.Sprintf("entry %d (%s): group must be >= 1", i, e))
		}
		key := TabletPlacement{Predicate: e.Predicate, Namespace: e.Namespace}
		if _, ok := seen[key]; ok {
			faults = append(faults, fmt.Sprintf("entry %d (%s): duplicate entry for this "+
				"namespace and predicate", i, e))
		}
		seen[key] = struct{}{}
	}
	if len(faults) > 0 {
		return nil, errors.Errorf("invalid tablet placement:\n\t%s", strings.Join(faults, "\n\t"))
	}
	return entries, nil
}

// ParseTabletPlacementFile reads and validates the tablet placement file at path.
func ParseTabletPlacementFile(path string) ([]TabletPlacement, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "opening tablet placement file %q", path)
	}
	defer func() {
		if err := f.Close(); err != nil {
			glog.Warningf("error closing tablet placement file %q: %v", path, err)
		}
	}()
	entries, err := ParseTabletPlacement(f)
	if err != nil {
		return nil, errors.Wrapf(err, "tablet placement file %q", path)
	}
	return entries, nil
}
