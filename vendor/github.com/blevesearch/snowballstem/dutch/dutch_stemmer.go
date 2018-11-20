//! This file was generated automatically by the Snowball to Go compiler
//! http://snowballstem.org/

package dutch

import (
	snowballRuntime "github.com/blevesearch/snowballstem"
)

var A_0 = []*snowballRuntime.Among{
	{Str: "", A: -1, B: 6, F: nil},
	{Str: "\u00E1", A: 0, B: 1, F: nil},
	{Str: "\u00E4", A: 0, B: 1, F: nil},
	{Str: "\u00E9", A: 0, B: 2, F: nil},
	{Str: "\u00EB", A: 0, B: 2, F: nil},
	{Str: "\u00ED", A: 0, B: 3, F: nil},
	{Str: "\u00EF", A: 0, B: 3, F: nil},
	{Str: "\u00F3", A: 0, B: 4, F: nil},
	{Str: "\u00F6", A: 0, B: 4, F: nil},
	{Str: "\u00FA", A: 0, B: 5, F: nil},
	{Str: "\u00FC", A: 0, B: 5, F: nil},
}

var A_1 = []*snowballRuntime.Among{
	{Str: "", A: -1, B: 3, F: nil},
	{Str: "I", A: 0, B: 2, F: nil},
	{Str: "Y", A: 0, B: 1, F: nil},
}

var A_2 = []*snowballRuntime.Among{
	{Str: "dd", A: -1, B: -1, F: nil},
	{Str: "kk", A: -1, B: -1, F: nil},
	{Str: "tt", A: -1, B: -1, F: nil},
}

var A_3 = []*snowballRuntime.Among{
	{Str: "ene", A: -1, B: 2, F: nil},
	{Str: "se", A: -1, B: 3, F: nil},
	{Str: "en", A: -1, B: 2, F: nil},
	{Str: "heden", A: 2, B: 1, F: nil},
	{Str: "s", A: -1, B: 3, F: nil},
}

var A_4 = []*snowballRuntime.Among{
	{Str: "end", A: -1, B: 1, F: nil},
	{Str: "ig", A: -1, B: 2, F: nil},
	{Str: "ing", A: -1, B: 1, F: nil},
	{Str: "lijk", A: -1, B: 3, F: nil},
	{Str: "baar", A: -1, B: 4, F: nil},
	{Str: "bar", A: -1, B: 5, F: nil},
}

var A_5 = []*snowballRuntime.Among{
	{Str: "aa", A: -1, B: -1, F: nil},
	{Str: "ee", A: -1, B: -1, F: nil},
	{Str: "oo", A: -1, B: -1, F: nil},
	{Str: "uu", A: -1, B: -1, F: nil},
}

var G_v = []byte{17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128}

var G_v_I = []byte{1, 0, 0, 17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128}

var G_v_j = []byte{17, 67, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128}

type Context struct {
	i_p2      int
	i_p1      int
	b_e_found bool
}

func r_prelude(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	var among_var int32
	// (, line 41
	// test, line 42
	var v_1 = env.Cursor
	// repeat, line 42
replab0:
	for {
		var v_2 = env.Cursor
	lab1:
		for range [2]struct{}{} {
			// (, line 42
			// [, line 43
			env.Bra = env.Cursor
			// substring, line 43
			among_var = env.FindAmong(A_0, context)
			if among_var == 0 {
				break lab1
			}
			// ], line 43
			env.Ket = env.Cursor
			if among_var == 0 {
				break lab1
			} else if among_var == 1 {
				// (, line 45
				// <-, line 45
				if !env.SliceFrom("a") {
					return false
				}
			} else if among_var == 2 {
				// (, line 47
				// <-, line 47
				if !env.SliceFrom("e") {
					return false
				}
			} else if among_var == 3 {
				// (, line 49
				// <-, line 49
				if !env.SliceFrom("i") {
					return false
				}
			} else if among_var == 4 {
				// (, line 51
				// <-, line 51
				if !env.SliceFrom("o") {
					return false
				}
			} else if among_var == 5 {
				// (, line 53
				// <-, line 53
				if !env.SliceFrom("u") {
					return false
				}
			} else if among_var == 6 {
				// (, line 54
				// next, line 54
				if env.Cursor >= env.Limit {
					break lab1
				}
				env.NextChar()
			}
			continue replab0
		}
		env.Cursor = v_2
		break replab0
	}
	env.Cursor = v_1
	// try, line 57
	var v_3 = env.Cursor
lab2:
	for {
		// (, line 57
		// [, line 57
		env.Bra = env.Cursor
		// literal, line 57
		if !env.EqS("y") {
			env.Cursor = v_3
			break lab2
		}
		// ], line 57
		env.Ket = env.Cursor
		// <-, line 57
		if !env.SliceFrom("Y") {
			return false
		}
		break lab2
	}
	// repeat, line 58
replab3:
	for {
		var v_4 = env.Cursor
	lab4:
		for range [2]struct{}{} {
			// goto, line 58
		golab5:
			for {
				var v_5 = env.Cursor
			lab6:
				for {
					// (, line 58
					if !env.InGrouping(G_v, 97, 232) {
						break lab6
					}
					// [, line 59
					env.Bra = env.Cursor
					// or, line 59
				lab7:
					for {
						var v_6 = env.Cursor
					lab8:
						for {
							// (, line 59
							// literal, line 59
							if !env.EqS("i") {
								break lab8
							}
							// ], line 59
							env.Ket = env.Cursor
							if !env.InGrouping(G_v, 97, 232) {
								break lab8
							}
							// <-, line 59
							if !env.SliceFrom("I") {
								return false
							}
							break lab7
						}
						env.Cursor = v_6
						// (, line 60
						// literal, line 60
						if !env.EqS("y") {
							break lab6
						}
						// ], line 60
						env.Ket = env.Cursor
						// <-, line 60
						if !env.SliceFrom("Y") {
							return false
						}
						break lab7
					}
					env.Cursor = v_5
					break golab5
				}
				env.Cursor = v_5
				if env.Cursor >= env.Limit {
					break lab4
				}
				env.NextChar()
			}
			continue replab3
		}
		env.Cursor = v_4
		break replab3
	}
	return true
}

func r_mark_regions(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	// (, line 64
	context.i_p1 = env.Limit
	context.i_p2 = env.Limit
	// gopast, line 69
golab0:
	for {
	lab1:
		for {
			if !env.InGrouping(G_v, 97, 232) {
				break lab1
			}
			break golab0
		}
		if env.Cursor >= env.Limit {
			return false
		}
		env.NextChar()
	}
	// gopast, line 69
golab2:
	for {
	lab3:
		for {
			if !env.OutGrouping(G_v, 97, 232) {
				break lab3
			}
			break golab2
		}
		if env.Cursor >= env.Limit {
			return false
		}
		env.NextChar()
	}
	// setmark p1, line 69
	context.i_p1 = env.Cursor
	// try, line 70
lab4:
	for {
		// (, line 70
		if !(context.i_p1 < 3) {
			break lab4
		}
		context.i_p1 = 3
		break lab4
	}
	// gopast, line 71
golab5:
	for {
	lab6:
		for {
			if !env.InGrouping(G_v, 97, 232) {
				break lab6
			}
			break golab5
		}
		if env.Cursor >= env.Limit {
			return false
		}
		env.NextChar()
	}
	// gopast, line 71
golab7:
	for {
	lab8:
		for {
			if !env.OutGrouping(G_v, 97, 232) {
				break lab8
			}
			break golab7
		}
		if env.Cursor >= env.Limit {
			return false
		}
		env.NextChar()
	}
	// setmark p2, line 71
	context.i_p2 = env.Cursor
	return true
}

func r_postlude(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	var among_var int32
	// repeat, line 75
replab0:
	for {
		var v_1 = env.Cursor
	lab1:
		for range [2]struct{}{} {
			// (, line 75
			// [, line 77
			env.Bra = env.Cursor
			// substring, line 77
			among_var = env.FindAmong(A_1, context)
			if among_var == 0 {
				break lab1
			}
			// ], line 77
			env.Ket = env.Cursor
			if among_var == 0 {
				break lab1
			} else if among_var == 1 {
				// (, line 78
				// <-, line 78
				if !env.SliceFrom("y") {
					return false
				}
			} else if among_var == 2 {
				// (, line 79
				// <-, line 79
				if !env.SliceFrom("i") {
					return false
				}
			} else if among_var == 3 {
				// (, line 80
				// next, line 80
				if env.Cursor >= env.Limit {
					break lab1
				}
				env.NextChar()
			}
			continue replab0
		}
		env.Cursor = v_1
		break replab0
	}
	return true
}

func r_R1(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	if !(context.i_p1 <= env.Cursor) {
		return false
	}
	return true
}

func r_R2(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	if !(context.i_p2 <= env.Cursor) {
		return false
	}
	return true
}

func r_undouble(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	// (, line 90
	// test, line 91
	var v_1 = env.Limit - env.Cursor
	// among, line 91
	if env.FindAmongB(A_2, context) == 0 {
		return false
	}
	env.Cursor = env.Limit - v_1
	// [, line 91
	env.Ket = env.Cursor
	// next, line 91
	if env.Cursor <= env.LimitBackward {
		return false
	}
	env.PrevChar()
	// ], line 91
	env.Bra = env.Cursor
	// delete, line 91
	if !env.SliceDel() {
		return false
	}
	return true
}

func r_e_ending(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	// (, line 94
	// unset e_found, line 95
	context.b_e_found = false
	// [, line 96
	env.Ket = env.Cursor
	// literal, line 96
	if !env.EqSB("e") {
		return false
	}
	// ], line 96
	env.Bra = env.Cursor
	// call R1, line 96
	if !r_R1(env, context) {
		return false
	}
	// test, line 96
	var v_1 = env.Limit - env.Cursor
	if !env.OutGroupingB(G_v, 97, 232) {
		return false
	}
	env.Cursor = env.Limit - v_1
	// delete, line 96
	if !env.SliceDel() {
		return false
	}
	// set e_found, line 97
	context.b_e_found = true
	// call undouble, line 98
	if !r_undouble(env, context) {
		return false
	}
	return true
}

func r_en_ending(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	// (, line 101
	// call R1, line 102
	if !r_R1(env, context) {
		return false
	}
	// and, line 102
	var v_1 = env.Limit - env.Cursor
	if !env.OutGroupingB(G_v, 97, 232) {
		return false
	}
	env.Cursor = env.Limit - v_1
	// not, line 102
	var v_2 = env.Limit - env.Cursor
lab0:
	for {
		// literal, line 102
		if !env.EqSB("gem") {
			break lab0
		}
		return false
	}
	env.Cursor = env.Limit - v_2
	// delete, line 102
	if !env.SliceDel() {
		return false
	}
	// call undouble, line 103
	if !r_undouble(env, context) {
		return false
	}
	return true
}

func r_standard_suffix(env *snowballRuntime.Env, ctx interface{}) bool {
	context := ctx.(*Context)
	_ = context
	var among_var int32
	// (, line 106
	// do, line 107
	var v_1 = env.Limit - env.Cursor
lab0:
	for {
		// (, line 107
		// [, line 108
		env.Ket = env.Cursor
		// substring, line 108
		among_var = env.FindAmongB(A_3, context)
		if among_var == 0 {
			break lab0
		}
		// ], line 108
		env.Bra = env.Cursor
		if among_var == 0 {
			break lab0
		} else if among_var == 1 {
			// (, line 110
			// call R1, line 110
			if !r_R1(env, context) {
				break lab0
			}
			// <-, line 110
			if !env.SliceFrom("heid") {
				return false
			}
		} else if among_var == 2 {
			// (, line 113
			// call en_ending, line 113
			if !r_en_ending(env, context) {
				break lab0
			}
		} else if among_var == 3 {
			// (, line 116
			// call R1, line 116
			if !r_R1(env, context) {
				break lab0
			}
			if !env.OutGroupingB(G_v_j, 97, 232) {
				break lab0
			}
			// delete, line 116
			if !env.SliceDel() {
				return false
			}
		}
		break lab0
	}
	env.Cursor = env.Limit - v_1
	// do, line 120
	var v_2 = env.Limit - env.Cursor
lab1:
	for {
		// call e_ending, line 120
		if !r_e_ending(env, context) {
			break lab1
		}
		break lab1
	}
	env.Cursor = env.Limit - v_2
	// do, line 122
	var v_3 = env.Limit - env.Cursor
lab2:
	for {
		// (, line 122
		// [, line 122
		env.Ket = env.Cursor
		// literal, line 122
		if !env.EqSB("heid") {
			break lab2
		}
		// ], line 122
		env.Bra = env.Cursor
		// call R2, line 122
		if !r_R2(env, context) {
			break lab2
		}
		// not, line 122
		var v_4 = env.Limit - env.Cursor
	lab3:
		for {
			// literal, line 122
			if !env.EqSB("c") {
				break lab3
			}
			break lab2
		}
		env.Cursor = env.Limit - v_4
		// delete, line 122
		if !env.SliceDel() {
			return false
		}
		// [, line 123
		env.Ket = env.Cursor
		// literal, line 123
		if !env.EqSB("en") {
			break lab2
		}
		// ], line 123
		env.Bra = env.Cursor
		// call en_ending, line 123
		if !r_en_ending(env, context) {
			break lab2
		}
		break lab2
	}
	env.Cursor = env.Limit - v_3
	// do, line 126
	var v_5 = env.Limit - env.Cursor
lab4:
	for {
		// (, line 126
		// [, line 127
		env.Ket = env.Cursor
		// substring, line 127
		among_var = env.FindAmongB(A_4, context)
		if among_var == 0 {
			break lab4
		}
		// ], line 127
		env.Bra = env.Cursor
		if among_var == 0 {
			break lab4
		} else if among_var == 1 {
			// (, line 129
			// call R2, line 129
			if !r_R2(env, context) {
				break lab4
			}
			// delete, line 129
			if !env.SliceDel() {
				return false
			}
			// or, line 130
		lab5:
			for {
				var v_6 = env.Limit - env.Cursor
			lab6:
				for {
					// (, line 130
					// [, line 130
					env.Ket = env.Cursor
					// literal, line 130
					if !env.EqSB("ig") {
						break lab6
					}
					// ], line 130
					env.Bra = env.Cursor
					// call R2, line 130
					if !r_R2(env, context) {
						break lab6
					}
					// not, line 130
					var v_7 = env.Limit - env.Cursor
				lab7:
					for {
						// literal, line 130
						if !env.EqSB("e") {
							break lab7
						}
						break lab6
					}
					env.Cursor = env.Limit - v_7
					// delete, line 130
					if !env.SliceDel() {
						return false
					}
					break lab5
				}
				env.Cursor = env.Limit - v_6
				// call undouble, line 130
				if !r_undouble(env, context) {
					break lab4
				}
				break lab5
			}
		} else if among_var == 2 {
			// (, line 133
			// call R2, line 133
			if !r_R2(env, context) {
				break lab4
			}
			// not, line 133
			var v_8 = env.Limit - env.Cursor
		lab8:
			for {
				// literal, line 133
				if !env.EqSB("e") {
					break lab8
				}
				break lab4
			}
			env.Cursor = env.Limit - v_8
			// delete, line 133
			if !env.SliceDel() {
				return false
			}
		} else if among_var == 3 {
			// (, line 136
			// call R2, line 136
			if !r_R2(env, context) {
				break lab4
			}
			// delete, line 136
			if !env.SliceDel() {
				return false
			}
			// call e_ending, line 136
			if !r_e_ending(env, context) {
				break lab4
			}
		} else if among_var == 4 {
			// (, line 139
			// call R2, line 139
			if !r_R2(env, context) {
				break lab4
			}
			// delete, line 139
			if !env.SliceDel() {
				return false
			}
		} else if among_var == 5 {
			// (, line 142
			// call R2, line 142
			if !r_R2(env, context) {
				break lab4
			}
			// Boolean test e_found, line 142
			if !context.b_e_found {
				break lab4
			}
			// delete, line 142
			if !env.SliceDel() {
				return false
			}
		}
		break lab4
	}
	env.Cursor = env.Limit - v_5
	// do, line 146
	var v_9 = env.Limit - env.Cursor
lab9:
	for {
		// (, line 146
		if !env.OutGroupingB(G_v_I, 73, 232) {
			break lab9
		}
		// test, line 148
		var v_10 = env.Limit - env.Cursor
		// (, line 148
		// among, line 149
		if env.FindAmongB(A_5, context) == 0 {
			break lab9
		}
		if !env.OutGroupingB(G_v, 97, 232) {
			break lab9
		}
		env.Cursor = env.Limit - v_10
		// [, line 152
		env.Ket = env.Cursor
		// next, line 152
		if env.Cursor <= env.LimitBackward {
			break lab9
		}
		env.PrevChar()
		// ], line 152
		env.Bra = env.Cursor
		// delete, line 152
		if !env.SliceDel() {
			return false
		}
		break lab9
	}
	env.Cursor = env.Limit - v_9
	return true
}

func Stem(env *snowballRuntime.Env) bool {
	var context = &Context{
		i_p2:      0,
		i_p1:      0,
		b_e_found: false,
	}
	_ = context
	// (, line 157
	// do, line 159
	var v_1 = env.Cursor
lab0:
	for {
		// call prelude, line 159
		if !r_prelude(env, context) {
			break lab0
		}
		break lab0
	}
	env.Cursor = v_1
	// do, line 160
	var v_2 = env.Cursor
lab1:
	for {
		// call mark_regions, line 160
		if !r_mark_regions(env, context) {
			break lab1
		}
		break lab1
	}
	env.Cursor = v_2
	// backwards, line 161
	env.LimitBackward = env.Cursor
	env.Cursor = env.Limit
	// do, line 162
	var v_3 = env.Limit - env.Cursor
lab2:
	for {
		// call standard_suffix, line 162
		if !r_standard_suffix(env, context) {
			break lab2
		}
		break lab2
	}
	env.Cursor = env.Limit - v_3
	env.Cursor = env.LimitBackward
	// do, line 163
	var v_4 = env.Cursor
lab3:
	for {
		// call postlude, line 163
		if !r_postlude(env, context) {
			break lab3
		}
		break lab3
	}
	env.Cursor = v_4
	return true
}
