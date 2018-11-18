//! This file was generated automatically by the Snowball to Go compiler
//! http://snowballstem.org/

package danish

import (
	snowballRuntime "github.com/blevesearch/snowballstem"
)

var A_0 = []*snowballRuntime.Among{
	{Str: "hed", A: -1, B: 1, F: nil},
	{Str: "ethed", A: 0, B: 1, F: nil},
	{Str: "ered", A: -1, B: 1, F: nil},
	{Str: "e", A: -1, B: 1, F: nil},
	{Str: "erede", A: 3, B: 1, F: nil},
	{Str: "ende", A: 3, B: 1, F: nil},
	{Str: "erende", A: 5, B: 1, F: nil},
	{Str: "ene", A: 3, B: 1, F: nil},
	{Str: "erne", A: 3, B: 1, F: nil},
	{Str: "ere", A: 3, B: 1, F: nil},
	{Str: "en", A: -1, B: 1, F: nil},
	{Str: "heden", A: 10, B: 1, F: nil},
	{Str: "eren", A: 10, B: 1, F: nil},
	{Str: "er", A: -1, B: 1, F: nil},
	{Str: "heder", A: 13, B: 1, F: nil},
	{Str: "erer", A: 13, B: 1, F: nil},
	{Str: "s", A: -1, B: 2, F: nil},
	{Str: "heds", A: 16, B: 1, F: nil},
	{Str: "es", A: 16, B: 1, F: nil},
	{Str: "endes", A: 18, B: 1, F: nil},
	{Str: "erendes", A: 19, B: 1, F: nil},
	{Str: "enes", A: 18, B: 1, F: nil},
	{Str: "ernes", A: 18, B: 1, F: nil},
	{Str: "eres", A: 18, B: 1, F: nil},
	{Str: "ens", A: 16, B: 1, F: nil},
	{Str: "hedens", A: 24, B: 1, F: nil},
	{Str: "erens", A: 24, B: 1, F: nil},
	{Str: "ers", A: 16, B: 1, F: nil},
	{Str: "ets", A: 16, B: 1, F: nil},
	{Str: "erets", A: 28, B: 1, F: nil},
	{Str: "et", A: -1, B: 1, F: nil},
	{Str: "eret", A: 30, B: 1, F: nil},
}

var A_1 = []*snowballRuntime.Among{
	{Str: "gd", A: -1, B: -1, F: nil},
	{Str: "dt", A: -1, B: -1, F: nil},
	{Str: "gt", A: -1, B: -1, F: nil},
	{Str: "kt", A: -1, B: -1, F: nil},
}

var A_2 = []*snowballRuntime.Among{
	{Str: "ig", A: -1, B: 1, F: nil},
	{Str: "lig", A: 0, B: 1, F: nil},
	{Str: "elig", A: 1, B: 1, F: nil},
	{Str: "els", A: -1, B: 1, F: nil},
	{Str: "l\u00F8st", A: -1, B: 2, F: nil},
}

var G_v = []byte{17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 48, 0, 128}

var G_s_ending = []byte{239, 254, 42, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 16}

type Context struct {
	i_x  int
	i_p1 int
	S_ch string
}

func r_mark_regions(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	// (, line 29
	context.i_p1 = env.Limit
	// test, line 33
	var v_1 = env.Cursor
	// (, line 33
	{
		// hop, line 33
		var c = env.ByteIndexForHop((3))
		if int32(0) > c || c > int32(env.Limit) {
			return false
		}
		env.Cursor = int(c)
	}
	// setmark x, line 33
	context.i_x = env.Cursor
	env.Cursor = v_1
	// goto, line 34
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
	// gopast, line 34
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
	// setmark p1, line 34
	context.i_p1 = env.Cursor
	// try, line 35
lab4:
	for {
		// (, line 35
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
	// (, line 40
	// setlimit, line 41
	var v_1 = env.Limit - env.Cursor
	// tomark, line 41
	if env.Cursor < context.i_p1 {
		return false
	}
	env.Cursor = context.i_p1
	var v_2 = env.LimitBackward
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit - v_1
	// (, line 41
	// [, line 41
	env.Ket = env.Cursor
	// substring, line 41
	among_var = env.FindAmongB(A_0, context)
	if among_var == 0 {
		env.LimitBackward = v_2
		return false
	}
	// ], line 41
	env.Bra = env.Cursor
	env.LimitBackward = v_2
	if among_var == 0 {
		return false
	} else if among_var == 1 {
		// (, line 48
		// delete, line 48
		if !env.SliceDel() {
			return false
		}
	} else if among_var == 2 {
		// (, line 50
		if !env.InGroupingB(G_s_ending, 97, 229) {
			return false
		}
		// delete, line 50
		if !env.SliceDel() {
			return false
		}
	}
	return true
}

func r_consonant_pair(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	// (, line 54
	// test, line 55
	var v_1 = env.Limit - env.Cursor
	// (, line 55
	// setlimit, line 56
	var v_2 = env.Limit - env.Cursor
	// tomark, line 56
	if env.Cursor < context.i_p1 {
		return false
	}
	env.Cursor = context.i_p1
	var v_3 = env.LimitBackward
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit - v_2
	// (, line 56
	// [, line 56
	env.Ket = env.Cursor
	// substring, line 56
	if env.FindAmongB(A_1, context) == 0 {
		env.LimitBackward = v_3
		return false
	}
	// ], line 56
	env.Bra = env.Cursor
	env.LimitBackward = v_3
	env.Cursor = env.Limit - v_1
	// next, line 62
	if env.Cursor <= env.LimitBackward {
		return false
	}
	env.PrevChar()
	// ], line 62
	env.Bra = env.Cursor
	// delete, line 62
	if !env.SliceDel() {
		return false
	}
	return true
}

func r_other_suffix(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	var among_var int32
	// (, line 65
	// do, line 66
	var v_1 = env.Limit - env.Cursor
lab0:
	for {
		// (, line 66
		// [, line 66
		env.Ket = env.Cursor
		// literal, line 66
		if !env.EqSB("st") {
			break lab0
		}
		// ], line 66
		env.Bra = env.Cursor
		// literal, line 66
		if !env.EqSB("ig") {
			break lab0
		}
		// delete, line 66
		if !env.SliceDel() {
			return false
		}
		break lab0
	}
	env.Cursor = env.Limit - v_1
	// setlimit, line 67
	var v_2 = env.Limit - env.Cursor
	// tomark, line 67
	if env.Cursor < context.i_p1 {
		return false
	}
	env.Cursor = context.i_p1
	var v_3 = env.LimitBackward
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit - v_2
	// (, line 67
	// [, line 67
	env.Ket = env.Cursor
	// substring, line 67
	among_var = env.FindAmongB(A_2, context)
	if among_var == 0 {
		env.LimitBackward = v_3
		return false
	}
	// ], line 67
	env.Bra = env.Cursor
	env.LimitBackward = v_3
	if among_var == 0 {
		return false
	} else if among_var == 1 {
		// (, line 70
		// delete, line 70
		if !env.SliceDel() {
			return false
		}
		// do, line 70
		var v_4 = env.Limit - env.Cursor
	lab1:
		for {
			// call consonant_pair, line 70
			if !r_consonant_pair(env, context) {
				break lab1
			}
			break lab1
		}
		env.Cursor = env.Limit - v_4
	} else if among_var == 2 {
		// (, line 72
		// <-, line 72
		if !env.SliceFrom("l\u00F8s") {
			return false
		}
	}
	return true
}

func r_undouble(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	// (, line 75
	// setlimit, line 76
	var v_1 = env.Limit - env.Cursor
	// tomark, line 76
	if env.Cursor < context.i_p1 {
		return false
	}
	env.Cursor = context.i_p1
	var v_2 = env.LimitBackward
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit - v_1
	// (, line 76
	// [, line 76
	env.Ket = env.Cursor
	if !env.OutGroupingB(G_v, 97, 248) {
		env.LimitBackward = v_2
		return false
	}
	// ], line 76
	env.Bra = env.Cursor
	// -> ch, line 76
	context.S_ch = env.SliceTo()
	if context.S_ch == "" {
		return false
	}
	env.LimitBackward = v_2
	// name ch, line 77
	if !env.EqSB(context.S_ch) {
		return false
	}
	// delete, line 78
	if !env.SliceDel() {
		return false
	}
	return true
}

func Stem(env *snowballRuntime.Env) bool {
	var context = &Context{
		i_x:  0,
		i_p1: 0,
		S_ch: "",
	}
	_ = context
	// (, line 82
	// do, line 84
	var v_1 = env.Cursor
lab0:
	for {
		// call mark_regions, line 84
		if !r_mark_regions(env, context) {
			break lab0
		}
		break lab0
	}
	env.Cursor = v_1
	// backwards, line 85
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit
	// (, line 85
	// do, line 86
	var v_2 = env.Limit - env.Cursor
lab1:
	for {
		// call main_suffix, line 86
		if !r_main_suffix(env, context) {
			break lab1
		}
		break lab1
	}
	env.Cursor = env.Limit - v_2
	// do, line 87
	var v_3 = env.Limit - env.Cursor
lab2:
	for {
		// call consonant_pair, line 87
		if !r_consonant_pair(env, context) {
			break lab2
		}
		break lab2
	}
	env.Cursor = env.Limit - v_3
	// do, line 88
	var v_4 = env.Limit - env.Cursor
lab3:
	for {
		// call other_suffix, line 88
		if !r_other_suffix(env, context) {
			break lab3
		}
		break lab3
	}
	env.Cursor = env.Limit - v_4
	// do, line 89
	var v_5 = env.Limit - env.Cursor
lab4:
	for {
		// call undouble, line 89
		if !r_undouble(env, context) {
			break lab4
		}
		break lab4
	}
	env.Cursor = env.Limit - v_5
	env.Cursor = env.LimitBackward
	return true
}
