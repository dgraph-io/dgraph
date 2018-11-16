//! This file was generated automatically by the Snowball to Go compiler
//! http://snowballstem.org/

package swedish

import (
	snowballRuntime "github.com/blevesearch/snowballstem"
)

var A_0 = []*snowballRuntime.Among{
	{Str: "a", A: -1, B: 1, F: nil},
	{Str: "arna", A: 0, B: 1, F: nil},
	{Str: "erna", A: 0, B: 1, F: nil},
	{Str: "heterna", A: 2, B: 1, F: nil},
	{Str: "orna", A: 0, B: 1, F: nil},
	{Str: "ad", A: -1, B: 1, F: nil},
	{Str: "e", A: -1, B: 1, F: nil},
	{Str: "ade", A: 6, B: 1, F: nil},
	{Str: "ande", A: 6, B: 1, F: nil},
	{Str: "arne", A: 6, B: 1, F: nil},
	{Str: "are", A: 6, B: 1, F: nil},
	{Str: "aste", A: 6, B: 1, F: nil},
	{Str: "en", A: -1, B: 1, F: nil},
	{Str: "anden", A: 12, B: 1, F: nil},
	{Str: "aren", A: 12, B: 1, F: nil},
	{Str: "heten", A: 12, B: 1, F: nil},
	{Str: "ern", A: -1, B: 1, F: nil},
	{Str: "ar", A: -1, B: 1, F: nil},
	{Str: "er", A: -1, B: 1, F: nil},
	{Str: "heter", A: 18, B: 1, F: nil},
	{Str: "or", A: -1, B: 1, F: nil},
	{Str: "s", A: -1, B: 2, F: nil},
	{Str: "as", A: 21, B: 1, F: nil},
	{Str: "arnas", A: 22, B: 1, F: nil},
	{Str: "ernas", A: 22, B: 1, F: nil},
	{Str: "ornas", A: 22, B: 1, F: nil},
	{Str: "es", A: 21, B: 1, F: nil},
	{Str: "ades", A: 26, B: 1, F: nil},
	{Str: "andes", A: 26, B: 1, F: nil},
	{Str: "ens", A: 21, B: 1, F: nil},
	{Str: "arens", A: 29, B: 1, F: nil},
	{Str: "hetens", A: 29, B: 1, F: nil},
	{Str: "erns", A: 21, B: 1, F: nil},
	{Str: "at", A: -1, B: 1, F: nil},
	{Str: "andet", A: -1, B: 1, F: nil},
	{Str: "het", A: -1, B: 1, F: nil},
	{Str: "ast", A: -1, B: 1, F: nil},
}

var A_1 = []*snowballRuntime.Among{
	{Str: "dd", A: -1, B: -1, F: nil},
	{Str: "gd", A: -1, B: -1, F: nil},
	{Str: "nn", A: -1, B: -1, F: nil},
	{Str: "dt", A: -1, B: -1, F: nil},
	{Str: "gt", A: -1, B: -1, F: nil},
	{Str: "kt", A: -1, B: -1, F: nil},
	{Str: "tt", A: -1, B: -1, F: nil},
}

var A_2 = []*snowballRuntime.Among{
	{Str: "ig", A: -1, B: 1, F: nil},
	{Str: "lig", A: 0, B: 1, F: nil},
	{Str: "els", A: -1, B: 1, F: nil},
	{Str: "fullt", A: -1, B: 3, F: nil},
	{Str: "l\u00F6st", A: -1, B: 2, F: nil},
}

var G_v = []byte{17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 24, 0, 32}

var G_s_ending = []byte{119, 127, 149}

type Context struct {
	i_x  int
	i_p1 int
}

func r_mark_regions(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	// (, line 26
	context.i_p1 = env.Limit
	// test, line 29
	var v_1 = env.Cursor
	// (, line 29
	{
		// hop, line 29
		var c = env.ByteIndexForHop((3))
		if int32(0) > c || c > int32(env.Limit) {
			return false
		}
		env.Cursor = int(c)
	}
	// setmark x, line 29
	context.i_x = env.Cursor
	env.Cursor = v_1
	// goto, line 30
golab0:
	for {
		var v_2 = env.Cursor
	lab1:
		for {
			if !env.InGrouping(G_v, 97, 246) {
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
	// gopast, line 30
golab2:
	for {
	lab3:
		for {
			if !env.OutGrouping(G_v, 97, 246) {
				break lab3
			}
			break golab2
		}
		if env.Cursor >= env.Limit {
			return false
		}
		env.NextChar()
	}
	// setmark p1, line 30
	context.i_p1 = env.Cursor
	// try, line 31
lab4:
	for {
		// (, line 31
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
	// (, line 36
	// setlimit, line 37
	var v_1 = env.Limit - env.Cursor
	// tomark, line 37
	if env.Cursor < context.i_p1 {
		return false
	}
	env.Cursor = context.i_p1
	var v_2 = env.LimitBackward
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit - v_1
	// (, line 37
	// [, line 37
	env.Ket = env.Cursor
	// substring, line 37
	among_var = env.FindAmongB(A_0, context)
	if among_var == 0 {
		env.LimitBackward = v_2
		return false
	}
	// ], line 37
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
		if !env.InGroupingB(G_s_ending, 98, 121) {
			return false
		}
		// delete, line 46
		if !env.SliceDel() {
			return false
		}
	}
	return true
}

func r_consonant_pair(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	// setlimit, line 50
	var v_1 = env.Limit - env.Cursor
	// tomark, line 50
	if env.Cursor < context.i_p1 {
		return false
	}
	env.Cursor = context.i_p1
	var v_2 = env.LimitBackward
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit - v_1
	// (, line 50
	// and, line 52
	var v_3 = env.Limit - env.Cursor
	// among, line 51
	if env.FindAmongB(A_1, context) == 0 {
		env.LimitBackward = v_2
		return false
	}
	env.Cursor = env.Limit - v_3
	// (, line 52
	// [, line 52
	env.Ket = env.Cursor
	// next, line 52
	if env.Cursor <= env.LimitBackward {
		env.LimitBackward = v_2
		return false
	}
	env.PrevChar()
	// ], line 52
	env.Bra = env.Cursor
	// delete, line 52
	if !env.SliceDel() {
		return false
	}
	env.LimitBackward = v_2
	return true
}

func r_other_suffix(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	var among_var int32
	// setlimit, line 55
	var v_1 = env.Limit - env.Cursor
	// tomark, line 55
	if env.Cursor < context.i_p1 {
		return false
	}
	env.Cursor = context.i_p1
	var v_2 = env.LimitBackward
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit - v_1
	// (, line 55
	// [, line 56
	env.Ket = env.Cursor
	// substring, line 56
	among_var = env.FindAmongB(A_2, context)
	if among_var == 0 {
		env.LimitBackward = v_2
		return false
	}
	// ], line 56
	env.Bra = env.Cursor
	if among_var == 0 {
		env.LimitBackward = v_2
		return false
	} else if among_var == 1 {
		// (, line 57
		// delete, line 57
		if !env.SliceDel() {
			return false
		}
	} else if among_var == 2 {
		// (, line 58
		// <-, line 58
		if !env.SliceFrom("l\u00F6s") {
			return false
		}
	} else if among_var == 3 {
		// (, line 59
		// <-, line 59
		if !env.SliceFrom("full") {
			return false
		}
	}
	env.LimitBackward = v_2
	return true
}

func Stem(env *snowballRuntime.Env) bool {
	var context = &Context{
		i_x:  0,
		i_p1: 0,
	}
	_ = context
	// (, line 64
	// do, line 66
	var v_1 = env.Cursor
lab0:
	for {
		// call mark_regions, line 66
		if !r_mark_regions(env, context) {
			break lab0
		}
		break lab0
	}
	env.Cursor = v_1
	// backwards, line 67
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit
	// (, line 67
	// do, line 68
	var v_2 = env.Limit - env.Cursor
lab1:
	for {
		// call main_suffix, line 68
		if !r_main_suffix(env, context) {
			break lab1
		}
		break lab1
	}
	env.Cursor = env.Limit - v_2
	// do, line 69
	var v_3 = env.Limit - env.Cursor
lab2:
	for {
		// call consonant_pair, line 69
		if !r_consonant_pair(env, context) {
			break lab2
		}
		break lab2
	}
	env.Cursor = env.Limit - v_3
	// do, line 70
	var v_4 = env.Limit - env.Cursor
lab3:
	for {
		// call other_suffix, line 70
		if !r_other_suffix(env, context) {
			break lab3
		}
		break lab3
	}
	env.Cursor = env.Limit - v_4
	env.Cursor = env.LimitBackward
	return true
}
