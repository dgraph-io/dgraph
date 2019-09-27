// Package sizeof provides utility functions for recursively computing the size
// of Go objects in memory, using the reflect package.
package sizeof

import (
	"math"
	"reflect"
)

// DeepSize reports the size of v in bytes, as reflect.Size, but also including
// all recursive substructures of v via maps, pointers, and slices. If v
// contains any cycles, the size of each pointer (re)introducing the cycle is
// included but the acyclic substructure is counted only once.
//
// Only values whose size and structure can be obtained by the reflect package
// are counted.  Some values have components that are not visible by
// reflection, and so are not counted or may be undercounted. In particular:
//
// The space occupied by code and data, reachable through variables captured in
// the closure of a function pointer, are not counted. A value of function type
// is counted only as a pointer.
//
// The unused buckets of a map cannot be inspected by the reflect package.
// Their size is estimated by assuming unfilled slots contain zeroes of their
// type.
//
// The unused capacity of the array under a slice is estimated by assuming the
// unused slots contain zeroes of their type. It is possible they contain non
// zero values from sharing or reslicing, but without explicitly reslicing the
// reflect package cannot touch them.
func DeepSize(v interface{}) int64 {
	return int64(valueSize(reflect.ValueOf(v), make(map[uintptr]bool)))
}

func valueSize(v reflect.Value, seen map[uintptr]bool) uintptr {
	base := v.Type().Size()
	switch v.Kind() {
	case reflect.Ptr:
		p := v.Pointer()
		if !seen[p] && !v.IsNil() {
			seen[p] = true
			return base + valueSize(v.Elem(), seen)
		}

	case reflect.Slice:
		n := v.Len()
		for i := 0; i < n; i++ {
			base += valueSize(v.Index(i), seen)
		}

		// Account for the parts of the array not covered by this slice.  Since
		// we can't get the values directly, assume they're zeroes. That may be
		// incorrect, in which case we may underestimate.
		if cap := v.Cap(); cap > n {
			base += v.Type().Size() * uintptr(cap-n)
		}

	case reflect.Map:
		// A map m has len(m) / 6.5 buckets, rounded up to a power of two, and
		// a minimum of one bucket. Each bucket is 16 bytes + 8*(keysize + valsize).
		//
		// We can't tell which keys are in which bucket by reflection, however,
		// so here we count the 16-byte header for each bucket, and then just add
		// in the computed key and value sizes.
		nb := uintptr(math.Pow(2, math.Ceil(math.Log(float64(v.Len())/6.5)/math.Log(2))))
		if nb == 0 {
			nb = 1
		}
		base = 16 * nb
		for _, key := range v.MapKeys() {
			base += valueSize(key, seen)
			base += valueSize(v.MapIndex(key), seen)
		}

		// We have nb buckets of 8 slots each, and v.Len() slots are filled.
		// The remaining slots we will assume contain zero key/value pairs.
		zk := v.Type().Key().Size()  // a zero key
		zv := v.Type().Elem().Size() // a zero value
		base += (8*nb - uintptr(v.Len())) * (zk + zv)

	case reflect.Struct:
		// Chase pointer and slice fields and add the size of their members.
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			switch f.Kind() {
			case reflect.Ptr:
				p := f.Pointer()
				if !seen[p] && !f.IsNil() {
					seen[p] = true
					base += valueSize(f.Elem(), seen)
				}
			case reflect.Slice:
				base += valueSize(f, seen)
			}
		}

	case reflect.String:
		return base + uintptr(v.Len())

	}
	return base
}
