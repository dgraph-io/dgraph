//! This file was generated automatically by the Snowball to Go compiler
//! http://snowballstem.org/

package norwegian

import (
	snowballRuntime "github.com/blevesearch/snowballstem"
)

var A_0 = []*snowballRuntime.Among{
	{Str: "a", A: -1, B: 1, F: nil},
	{Str: "e", A: -1, B: 1, F: nil},
	{Str: "ede", A: 1, B: 1, F: nil},
	{Str: "ande", A: 1, B: 1, F: nil},
	{Str: "ende", A: 1, B: 1, F: nil},
	{Str: "ane", A: 1, B: 1, F: nil},
	{Str: "ene", A: 1, B: 1, F: nil},
	{Str: "hetene", A: 6, B: 1, F: nil},
	{Str: "erte", A: 1, B: 3, F: nil},
	{Str: "en", A: -1, B: 1, F: nil},
	{Str: "heten", A: 9, B: 1, F: nil},
	{Str: "ar", A: -1, B: 1, F: nil},
	{Str: "er", A: -1, B: 1, F: nil},
	{Str: "heter", A: 12, B: 1, F: nil},
	{Str: "s", A: -1, B: 2, F: nil},
	{Str: "as", A: 14, B: 1, F: nil},
	{Str: "es", A: 14, B: 1, F: nil},
	{Str: "edes", A: 16, B: 1, F: nil},
	{Str: "endes", A: 16, B: 1, F: nil},
	{Str: "enes", A: 16, B: 1, F: nil},
	{Str: "hetenes", A: 19, B: 1, F: nil},
	{Str: "ens", A: 14, B: 1, F: nil},
	{Str: "hetens", A: 21, B: 1, F: nil},
	{Str: "ers", A: 14, B: 1, F: nil},
	{Str: "ets", A: 14, B: 1, F: nil},
	{Str: "et", A: -1, B: 1, F: nil},
	{Str: "het", A: 25, B: 1, F: nil},
	{Str: "ert", A: -1, B: 3, F: nil},
	{Str: "ast", A: -1, B: 1, F: nil},
}

var A_1 = []*snowballRuntime.Among{
	{Str: "dt", A: -1, B: -1, F: nil},
	{Str: "vt", A: -1, B: -1, F: nil},
}

var A_2 = []*snowballRuntime.Among{
	{Str: "leg", A: -1, B: 1, F: nil},
	{Str: "eleg", A: 0, B: 1, F: nil},
	{Str: "ig", A: -1, B: 1, F: nil},
	{Str: "eig", A: 2, B: 1, F: nil},
	{Str: "lig", A: 2, B: 1, F: nil},
	{Str: "elig", A: 4, B: 1, F: nil},
	{Str: "els", A: -1, B: 1, F: nil},
	{Str: "lov", A: -1, B: 1, F: nil},
	{Str: "elov", A: 7, B: 1, F: nil},
	{Str: "slov", A: 7, B: 1, F: nil},
	{Str: "hetslov", A: 9, B: 1, F: nil},
}

var G_v = []byte{17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 48, 0, 128}

var G_s_ending = []byte{119, 125, 149, 1}

type Context struct {
	i_x  int
	i_p1 int
}

func r_mark_regions(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	// (, line 26
	context.i_p1 = env.Limit
	// test, line 30
	var v_1 = env.Cursor
	// (, line 30
	{
		// hop, line 30
		var c = env.ByteIndexForHop((3))
		if int32(0) > c || c > int32(env.Limit) {
			return false
		}
		env.Cursor = int(c)
	}
	// setmark x, line 30
	context.i_x = env.Cursor
	env.Cursor = v_1
	// goto, line 31
golab0:
	for {
		var v_2 = env.Cursor
	lab1:
		for {
			if !env.InGrouping(G_v, 97, 248) {
				break lab1
			}
			env.Cursor = v_2
			break golab0
		}
		env.Cursor = v_2
		if env.Cursor >= env.Limit {
			return false
		}
		env.NextChar()
	}
	// gopast, line 31
golab2:
	for {
	lab3:
		for {
			if !env.OutGrouping(G_v, 97, 248) {
				break lab3
			}
			break golab2
		}
		if env.Cursor >= env.Limit {
			return false
		}
		env.NextChar()
	}
	// setmark p1, line 31
	context.i_p1 = env.Cursor
	// try, line 32
lab4:
	for {
		// (, line 32
		if !(context.i_p1 < context.i_x) {
			break lab4
		}
		context.i_p1 = context.i_x
		break lab4
	}
	return true
}

func r_main_suffix(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	var among_var int32
	// (, line 37
	// setlimit, line 38
	var v_1 = env.Limit - env.Cursor
	// tomark, line 38
	if env.Cursor < context.i_p1 {
		return false
	}
	env.Cursor = context.i_p1
	var v_2 = env.LimitBackward
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit - v_1
	// (, line 38
	// [, line 38
	env.Ket = env.Cursor
	// substring, line 38
	among_var = env.FindAmongB(A_0, context)
	if among_var == 0 {
		env.LimitBackward = v_2
		return false
	}
	// ], line 38
	env.Bra = env.Cursor
	env.LimitBackward = v_2
	if among_var == 0 {
		return false
	} else if among_var == 1 {
		// (, line 44
		// delete, line 44
		if !env.SliceDel() {
			return false
		}
	} else if among_var == 2 {
		// (, line 46
		// or, line 46
	lab0:
		for {
			var v_3 = env.Limit - env.Cursor
		lab1:
			for {
				if !env.InGroupingB(G_s_ending, 98, 122) {
					break lab1
				}
				break lab0
			}
			env.Cursor = env.Limit - v_3
			// (, line 46
			// literal, line 46
			if !env.EqSB("k") {
				return false
			}
			if !env.OutGroupingB(G_v, 97, 248) {
				return false
			}
			break lab0
		}
		// delete, line 46
		if !env.SliceDel() {
			return false
		}
	} else if among_var == 3 {
		// (, line 48
		// <-, line 48
		if !env.SliceFrom("er") {
			return false
		}
	}
	return true
}

func r_consonant_pair(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	// (, line 52
	// test, line 53
	var v_1 = env.Limit - env.Cursor
	// (, line 53
	// setlimit, line 54
	var v_2 = env.Limit - env.Cursor
	// tomark, line 54
	if env.Cursor < context.i_p1 {
		return false
	}
	env.Cursor = context.i_p1
	var v_3 = env.LimitBackward
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit - v_2
	// (, line 54
	// [, line 54
	env.Ket = env.Cursor
	// substring, line 54
	if env.FindAmongB(A_1, context) == 0 {
		env.LimitBackward = v_3
		return false
	}
	// ], line 54
	env.Bra = env.Cursor
	env.LimitBackward = v_3
	env.Cursor = env.Limit - v_1
	// next, line 59
	if env.Cursor <= env.LimitBackward {
		return false
	}
	env.PrevChar()
	// ], line 59
	env.Bra = env.Cursor
	// delete, line 59
	if !env.SliceDel() {
		return false
	}
	return true
}

func r_other_suffix(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	var among_var int32
	// (, line 62
	// setlimit, line 63
	var v_1 = env.Limit - env.Cursor
	// tomark, line 63
	if env.Cursor < context.i_p1 {
		return false
	}
	env.Cursor = context.i_p1
	var v_2 = env.LimitBackward
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit - v_1
	// (, line 63
	// [, line 63
	env.Ket = env.Cursor
	// substring, line 63
	among_var = env.FindAmongB(A_2, context)
	if among_var == 0 {
		env.LimitBackward = v_2
		return false
	}
	// ], line 63
	env.Bra = env.Cursor
	env.LimitBackward = v_2
	if among_var == 0 {
		return false
	} else if among_var == 1 {
		// (, line 67
		// delete, line 67
		if !env.SliceDel() {
			return false
		}
	}
	return true
}

func Stem(env *snowballRuntime.Env) bool {
	var context = &Context{
		i_x:  0,
		i_p1: 0,
	}
	_ = context
	// (, line 72
	// do, line 74
	var v_1 = env.Cursor
lab0:
	for {
		// call mark_regions, line 74
		if !r_mark_regions(env, context) {
			break lab0
		}
		break lab0
	}
	env.Cursor = v_1
	// backwards, line 75
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit
	// (, line 75
	// do, line 76
	var v_2 = env.Limit - env.Cursor
lab1:
	for {
		// call main_suffix, line 76
		if !r_main_suffix(env, context) {
			break lab1
		}
		break lab1
	}
	env.Cursor = env.Limit - v_2
	// do, line 77
	var v_3 = env.Limit - env.Cursor
lab2:
	for {
		// call consonant_pair, line 77
		if !r_consonant_pair(env, context) {
			break lab2
		}
		break lab2
	}
	env.Cursor = env.Limit - v_3
	// do, line 78
	var v_4 = env.Limit - env.Cursor
lab3:
	for {
		// call other_suffix, line 78
		if !r_other_suffix(env, context) {
			break lab3
		}
		break lab3
	}
	env.Cursor = env.Limit - v_4
	env.Cursor = env.LimitBackward
	return true
}
