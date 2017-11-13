/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package gql

import (
	"os"
	"runtime/debug"
	"testing"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/stretchr/testify/require"
)

func childAttrs(g *GraphQuery) []string {
	var out []string
	for _, c := range g.Children {
		out = append(out, c.Attr)
	}
	return out
}

func TestParseCountValError(t *testing.T) {
	query := `
{
  me(func: uid(1)) {
    Upvote {
       u as Author
    }
    count(val(u))
  }
}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "count of a variable is not allowed")
}

func TestParseVarError(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			a as friends
		}

		me(func: uid(a)) {
			uid(a)
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot do uid() of a variable")
}

func TestParseQueryListPred1(t *testing.T) {
	query := `
	{
		var(func: uid( 0x0a)) {
			friends {
				expand(_all_)
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestParseQueryAliasListPred(t *testing.T) {
	query := `
	{
		me(func: uid(0x0a)) {
			pred: _predicate_
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, "pred", res.Query[0].Children[0].Alias)
	require.Equal(t, "_predicate_", res.Query[0].Children[0].Attr)
}

func TestParseQueryCountListPred(t *testing.T) {
	query := `
	{
		me(func: uid(0x0a)) {
			count(_predicate_)
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, true, res.Query[0].Children[0].IsCount)
	require.Equal(t, "_predicate_", res.Query[0].Children[0].Attr)
}

func TestParseQueryListPred2(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			f as friends
		}

		var(func: uid(f)) {
			l as _predicate_
		}

		var(func: uid( 0x0a)) {
			friends {
				expand(val(l))
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestParseQueryListPred_MultiVarError(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			f as friends
		}

		var(func: uid(f)) {
			l as _predicate_
			friend {
				g as _predicate_
			}
		}

		var(func: uid( 0x0a)) {
			friends {
				expand(val(l, g))
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	// Only one variable allowed in expand.
	require.Error(t, err)
	require.Contains(t, err.Error(), "Exactly one variable expected")
}

func TestParseQueryWithNoVarValError(t *testing.T) {
	query := `
	{
		me(func: uid(), orderasc: val(n)) {
			name
		}

		var(func: uid(0x0a)) {
			friends {
				n AS name
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestParseQueryAggChild(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			min(friends) {
				name
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only variables allowed in aggregate functions")
}

func TestParseQueryWithXIDError(t *testing.T) {
	query := `
{
      me(func: uid(aliceInWonderland)) {
        type
        writtenIn
        name
        character {
                name
        }
        author {
                name
                born
                died
        }
      }
    }`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Some variables are used but not defined")
	require.Contains(t, err.Error(), "Used:[aliceInWonderland]")
}

func TestParseQueryWithMultiVarValError(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(n, d)) {
			name
		}

		var(func: uid(0x0a)) {
			L AS friends {
				n AS name
				d as age
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected only one variable but got: 2")
}

func TestParseQueryWithVarValAggErr(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(c)) {
			name
		}

		var(func: uid(0x0a)) {
			L as friends {
				a as age
				c as sumvar()
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected argument but got ')'")
}

func TestParseQueryWithVarValAgg_Error1(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(d)) {
			name
		}

		var(func: uid(0x0a)) {
			L as friends {
				a as age
				b as count(friends)
				c as count(relatives)
				d as math(a + b*c + exp())
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Empty () not allowed in math block")
}

func TestParseQueryWithVarValAgg_Error2(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(d)) {
			name
		}

		var(func: uid(0x0a)) {
			L as friends {
				a as age
				b as count(friends)
				c as count(relatives)
				d as math(a + b*c+ log())
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unknown math function: log")
}

func TestParseQueryWithVarValAgg_Error3(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(d)) {
			name
			val(f)
		}

		var(func: uid(0x0a)) {
			L as friends {
				a as age
				b as count(friends)
				c as count(relatives)
				d as math(a + b*c)
				f as math()
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Empty () not allowed in math block")
}
func TestParseQueryWithVarValAggNested(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(d)) {
			name
		}

		var(func: uid(0x0a)) {
			L as friends {
				a as age
				b as count(friends)
				c as count(relatives)
				d as math(a + b*c)
			}
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.EqualValues(t, "(+ a (* b c))",
		res.Query[1].Children[0].Children[3].MathExp.debugString())
	require.NoError(t, err)
}

func TestParseQueryWithVarValAggNested2(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(d)) {
			name
			val(q)
		}

		var(func: uid(0x0a)) {
			L as friends {
				a as age
				b as count(friends)
				c as count(relatives)
				d as math(exp(a + b + 1) - ln(c))
				q as math(c*-1+-b+(-b*c))
			}
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.EqualValues(t, "(- (exp (+ (+ a b) 1E+00)) (ln c))",
		res.Query[1].Children[0].Children[3].MathExp.debugString())
	require.EqualValues(t, "(+ (+ (* c (u- 1E+00)) (u- b)) (* (u- b) c))",
		res.Query[1].Children[0].Children[4].MathExp.debugString())
}

func TestParseQueryWithVarValAggNested4(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(d) ) {
			name
		}

		var(func: uid(0x0a)) {
			L as friends {
				a as age
				b as count(friends)
				c as count(relatives)
				d as math(exp(a + b + 1) - max(c,ln(c)) + sqrt(a%b))
			}
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.EqualValues(t, "(+ (- (exp (+ (+ a b) 1E+00)) (max c (ln c))) (sqrt (% a b)))",
		res.Query[1].Children[0].Children[3].MathExp.debugString())
}

func TestParseQueryWithVarValAggLogSqrt(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(d) ) {
			name
			val(e)
		}

		var(func: uid(0x0a)) {
			L as friends {
				a as age
				d as math(ln(sqrt(a)))
				e as math(sqrt(ln(a)))
			}
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.EqualValues(t, "(ln (sqrt a))",
		res.Query[1].Children[0].Children[1].MathExp.debugString())
	require.EqualValues(t, "(sqrt (ln a))",
		res.Query[1].Children[0].Children[2].MathExp.debugString())
}

func TestParseQueryWithVarValAggNestedConditional(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(d) ) {
			name
			val(f)
		}

		var(func: uid(0x0a)) {
			L as friends {
				a as age
				b as count(friends)
				c as count(relatives)
				d as math(cond(a <= 10, exp(a + b + 1), ln(c)) + 10*a)
				e as math(cond(a!=10, exp(a + b + 1), ln(d)))
				f as math(cond(a==10, exp(a + b + 1), ln(e)))
			}
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.EqualValues(t, "(+ (cond (<= a 1E+01) (exp (+ (+ a b) 1E+00)) (ln c)) (* 1E+01 a))",
		res.Query[1].Children[0].Children[3].MathExp.debugString())
	require.EqualValues(t, "(cond (!= a 1E+01) (exp (+ (+ a b) 1E+00)) (ln d))",
		res.Query[1].Children[0].Children[4].MathExp.debugString())
	require.EqualValues(t, "(cond (== a 1E+01) (exp (+ (+ a b) 1E+00)) (ln e))",
		res.Query[1].Children[0].Children[5].MathExp.debugString())
}

func TestParseQueryWithVarValAggNested3(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(d) ) {
			name
	}

		var(func: uid(0x0a)) {
			L as friends {
				a as age
				b as count(friends)
				c as count(relatives)
				d as math(a + b * c / a + exp(a + b + 1) - ln(c))
			}
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.EqualValues(t, "(+ (+ a (* b (/ c a))) (- (exp (+ (+ a b) 1E+00)) (ln c)))",
		res.Query[1].Children[0].Children[3].MathExp.debugString())
}

func TestParseQueryWithVarValAggNested_Error1(t *testing.T) {
	// No args to mulvar.
	query := `
	{
		me(func: uid(L), orderasc: val(d)) {
			name
		}

		var(func: uid(0x0a)) {
			L as friends {
				a as age
				d as math(a + *)
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected 2 operands")
}

func TestParseQueryWithVarValAggNested_Error2(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(d)) {
			name
		}

		var(func: uid(0x0a)) {
			L as friends {
				a as age
				b as count(friends)
				c as count(relatives)
				d as math(a +b*c -)
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected 2 operands")
}

func TestParseQueryWithLevelAgg(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			friends {
				a as count(age)
			}
			s as sum(val(a))
		}

		sumage(func: uid( 0x0a)) {
			val(s)
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "a", res.Query[0].Children[0].Children[0].Var)
	require.True(t, res.Query[0].Children[1].IsInternal)
	require.Equal(t, "a", res.Query[0].Children[1].NeedsVar[0].Name)
	require.Equal(t, VALUE_VAR, res.Query[0].Children[1].NeedsVar[0].Typ)
	require.Equal(t, "s", res.Query[0].Children[1].Var)
}

func TestParseQueryWithVarValAggCombination(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(c) ) {
			name
			val(c)
		}

		var(func: uid(0x0a)) {
			L as friends {
				x as age
			}
			a as min(val(x))
			b as max(val(x))
			c as math(a + b)
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0].Name)
	require.Equal(t, UID_VAR, res.Query[0].NeedsVar[0].Typ)
	require.Equal(t, "c", res.Query[0].NeedsVar[1].Name)
	require.Equal(t, VALUE_VAR, res.Query[0].NeedsVar[1].Typ)
	require.Equal(t, "c", res.Query[0].Order[0].Attr)
	require.Equal(t, "name", res.Query[0].Children[0].Attr)
	require.Equal(t, "val", res.Query[0].Children[1].Attr)
	require.Equal(t, 1, len(res.Query[0].Children[1].NeedsVar))
	require.Equal(t, "c", res.Query[0].Children[1].NeedsVar[0].Name)
	require.Equal(t, "L", res.Query[1].Children[0].Var)
	require.Equal(t, "a", res.Query[1].Children[1].Var)
	require.Equal(t, "b", res.Query[1].Children[2].Var)
	require.Equal(t, "c", res.Query[1].Children[3].Var)
	require.NotNil(t, res.Query[1].Children[3].MathExp)
	require.Equal(t, "+", res.Query[1].Children[3].MathExp.Fn)
	require.Equal(t, "a", res.Query[1].Children[3].MathExp.Child[0].Var)
	require.Equal(t, "b", res.Query[1].Children[3].MathExp.Child[1].Var)
}

func TestParseQueryWithVarValAgg(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(n) ) {
			name
		}

		var(func: uid(0x0a)) {
			L AS friends {
				na as name
			}
			n as min(val(na))
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0].Name)
	require.Equal(t, UID_VAR, res.Query[0].NeedsVar[0].Typ)
	require.Equal(t, "n", res.Query[0].NeedsVar[1].Name)
	require.Equal(t, VALUE_VAR, res.Query[0].NeedsVar[1].Typ)
	require.Equal(t, "n", res.Query[0].Order[0].Attr)
	require.Equal(t, "name", res.Query[0].Children[0].Attr)
	require.Equal(t, "L", res.Query[1].Children[0].Var)
	require.Equal(t, "na", res.Query[1].Children[0].Children[0].Var)
	require.Equal(t, "n", res.Query[1].Children[1].Var)
	require.Equal(t, "min", res.Query[1].Children[1].Func.Name)
}

func TestParseQueryWithVarValAggError(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: uid(n)) {
			name
		}

		var(func: uid(0x0a)) {
			L AS friends {
				na as name
			}
			n as min(val(na))
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected val(). Got uid() with order.")
}

func TestParseQueryWithVarValAggError2(t *testing.T) {
	query := `
	{
		me(func: val(L), orderasc: val(n)) {
			name
		}

		var(func: uid(0x0a)) {
			L AS friends {
				na as name
			}
			n as min(val(na))
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Function name: val is not valid.")
}

func TestParseQueryWithVarValCount(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(n) ) {
			name
		}

		var(func: uid(0x0a)) {
			L AS friends {
				n AS count(friend)
			}
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0].Name)
	require.Equal(t, UID_VAR, res.Query[0].NeedsVar[0].Typ)
	require.Equal(t, "n", res.Query[0].NeedsVar[1].Name)
	require.Equal(t, VALUE_VAR, res.Query[0].NeedsVar[1].Typ)
	require.Equal(t, "n", res.Query[0].Order[0].Attr)
	require.Equal(t, "name", res.Query[0].Children[0].Attr)
	require.Equal(t, "L", res.Query[1].Children[0].Var)
	require.True(t, res.Query[1].Children[0].Children[0].IsCount)
}

func TestParseQueryWithVarVal(t *testing.T) {
	query := `
	{
		me(func: uid(L), orderasc: val(n) ) {
			name
		}

		var(func: uid(0x0a)) {
			L AS friends {
				n AS name
			}
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0].Name)
	require.Equal(t, UID_VAR, res.Query[0].NeedsVar[0].Typ)
	require.Equal(t, "n", res.Query[0].NeedsVar[1].Name)
	require.Equal(t, VALUE_VAR, res.Query[0].NeedsVar[1].Typ)
	require.Equal(t, "n", res.Query[0].Order[0].Attr)
	require.Equal(t, "name", res.Query[0].Children[0].Attr)
	require.Equal(t, "L", res.Query[1].Children[0].Var)
	require.Equal(t, "n", res.Query[1].Children[0].Children[0].Var)
}

func TestParseQueryWithVarMultiRoot(t *testing.T) {
	query := `
	{
		me(func: uid( L, J, K)) {name}
		var(func: uid(0x0a)) {L AS friends}
		var(func: uid(0x0a)) {J AS friends}
		var(func: uid(0x0a)) {K AS friends}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 4, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0].Name)
	require.Equal(t, "J", res.Query[0].NeedsVar[1].Name)
	require.Equal(t, "K", res.Query[0].NeedsVar[2].Name)
	require.Equal(t, UID_VAR, res.Query[0].NeedsVar[0].Typ)
	require.Equal(t, UID_VAR, res.Query[0].NeedsVar[1].Typ)
	require.Equal(t, UID_VAR, res.Query[0].NeedsVar[2].Typ)
	require.Equal(t, "L", res.Query[1].Children[0].Var)
	require.Equal(t, "J", res.Query[2].Children[0].Var)
	require.Equal(t, "K", res.Query[3].Children[0].Var)
}

func TestParseQueryWithVar(t *testing.T) {
	query := `
	{
		me(func: uid(L)) {name}
		him(func: uid(J)) {name}
		you(func: uid(K)) {name}
		var(func: uid(0x0a)) {L AS friends}
		var(func: uid(0x0a)) {J AS friends}
		var(func: uid(0x0a)) {K AS friends}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 6, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0].Name)
	require.Equal(t, "J", res.Query[1].NeedsVar[0].Name)
	require.Equal(t, "K", res.Query[2].NeedsVar[0].Name)
	require.Equal(t, UID_VAR, res.Query[0].NeedsVar[0].Typ)
	require.Equal(t, UID_VAR, res.Query[1].NeedsVar[0].Typ)
	require.Equal(t, UID_VAR, res.Query[2].NeedsVar[0].Typ)
	require.Equal(t, "L", res.Query[3].Children[0].Var)
	require.Equal(t, "J", res.Query[4].Children[0].Var)
	require.Equal(t, "K", res.Query[5].Children[0].Var)
}

func TestParseQueryWithVarError1(t *testing.T) {
	query := `
	{
		him(func: uid(J)) {name}
		you(func: uid(K)) {name}
		var(func: uid(0x0a)) {L AS friends}
		var(func: uid(0x0a)) {J AS friends}
		var(func: uid(0x0a)) {K AS friends}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Some variables are defined but not used")
}

func TestParseQueryWithVarError2(t *testing.T) {
	query := `
	{
		me(func: uid(L)) {name}
		him(func: uid(J)) {name}
		you(func: uid(K)) {name}
		var(func: uid(0x0a)) {L AS friends}
		var(func: uid(0x0a)) {K AS friends}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Some variables are used but not defined")
}

func TestParseQueryFilterError1A(t *testing.T) {
	query := `
	{
		me(func: uid(1) @filter(anyof(name, "alice"))) {
		 name
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "\"@\"")
}

func TestParseQueryFilterError1B(t *testing.T) {
	query := `
	{
		me(func: uid(1)) @filter(anyofterms(name"alice")) {
		 name
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected comma or language but got: \"alice\"")
}

func TestParseQueryFilterError2(t *testing.T) {
	query := `
	{
		me(func: uid(1)) @filter(anyofterms(name "alice")) {
		 name
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected comma or language but got: \"alice\"")
}

func TestParseQueryWithVarAtRootFilterID(t *testing.T) {
	query := `
	{
		K as var(func: uid(0x0a)) {
			L AS friends
		}
		me(func: uid(K)) @filter(uid(L)) {
		 name
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "K", res.Query[0].Var)
	require.Equal(t, "L", res.Query[0].Children[0].Var)
	require.Equal(t, "L", res.Query[1].Filter.Func.NeedsVar[0].Name)
	require.Equal(t, UID_VAR, res.Query[1].Filter.Func.NeedsVar[0].Typ)
	require.Equal(t, []string{"K", "L"}, res.QueryVars[0].Defines)
}

func TestParseQueryWithVarAtRoot(t *testing.T) {
	query := `
	{
		K AS var(func: uid(0x0a)) {
			fr as friends
		}
		me(func: uid(fr)) @filter(uid(K)) {
		 name	@filter(uid(fr))
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "K", res.Query[0].Var)
	require.Equal(t, "fr", res.Query[0].Children[0].Var)
	require.Equal(t, "fr", res.Query[1].NeedsVar[0].Name)
	require.Equal(t, UID_VAR, res.Query[1].NeedsVar[0].Typ)
	require.Equal(t, []string{"K", "fr"}, res.QueryVars[0].Defines)
}

func TestParseQueryWithVarInIneqError(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			fr as friends {
				a as age
			}
		}

		me(func: uid(fr)) @filter(gt(val(a, b), 10)) {
		 name
		}
	}
`
	// Multiple vars not allowed.
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Multiple variables not allowed in a function")
}

func TestParseQueryWithVarInIneq(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			fr as friends {
				a as age
			}
		}

		me(func: uid(fr)) @filter(gt(val(a), 10)) {
		 name
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "fr", res.Query[0].Children[0].Var)
	require.Equal(t, "fr", res.Query[1].NeedsVar[0].Name)
	require.Equal(t, UID_VAR, res.Query[1].NeedsVar[0].Typ)
	require.Equal(t, VALUE_VAR, res.Query[1].Filter.Func.NeedsVar[0].Typ)
	require.Equal(t, 1, len(res.Query[1].Filter.Func.Args))
	require.Equal(t, "a", res.Query[1].Filter.Func.Attr)
	require.Equal(t, true, res.Query[1].Filter.Func.IsValueVar)
	require.Equal(t, "10", res.Query[1].Filter.Func.Args[0].Value)
	require.Equal(t, false, res.Query[1].Filter.Func.Args[0].IsValueVar)
	require.Equal(t, "gt", res.Query[1].Filter.Func.Name)
}

func TestParseQueryWithVar1(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			L AS friends
		}

		me(func: uid(L)) {
		 name
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "L", res.Query[0].Children[0].Var)
	require.Equal(t, "L", res.Query[1].NeedsVar[0].Name)
	require.Equal(t, UID_VAR, res.Query[1].NeedsVar[0].Typ)
}

func TestParseQueryWithMultipleVar(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			L AS friends {
				B AS relatives
			}
		}

		me(func: uid(L)) {
		 name
		}

		relatives(func: uid(B)) {
			name
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 3, len(res.Query))
	require.Equal(t, "L", res.Query[0].Children[0].Var)
	require.Equal(t, "B", res.Query[0].Children[0].Children[0].Var)
	require.Equal(t, "L", res.Query[1].NeedsVar[0].Name)
	require.Equal(t, "B", res.Query[2].NeedsVar[0].Name)
	require.Equal(t, UID_VAR, res.Query[1].NeedsVar[0].Typ)
	require.Equal(t, UID_VAR, res.Query[2].NeedsVar[0].Typ)
	require.Equal(t, []string{"L", "B"}, res.QueryVars[0].Defines)
	require.Equal(t, []string{"L"}, res.QueryVars[1].Needs)
	require.Equal(t, []string{"B"}, res.QueryVars[2].Needs)
}

func TestParseShortestPath(t *testing.T) {
	query := `
	{
		shortest(from:0x0a, to:0x0b, numpaths: 3) {
			friends
			name
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, "0x0a", res.Query[0].Args["from"])
	require.Equal(t, "0x0b", res.Query[0].Args["to"])
	require.Equal(t, "3", res.Query[0].Args["numpaths"])
}

func TestParseMultipleQueries(t *testing.T) {
	query := `
	{
		you(func: uid(0x0a)) {
			name
		}

		me(func: uid(0x0b)) {
		 friends
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
}

func TestParseRootArgs1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a), first: -4, offset: +1) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, 2, len(res.Query[0].Args))
	require.Equal(t, "-4", res.Query[0].Args["first"])
	require.Equal(t, "+1", res.Query[0].Args["offset"])
	require.Equal(t, childAttrs(res.Query[0]), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(res.Query[0].Children[0]), []string{"name"})
}

func TestParseRootArgs2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a), first: 1, offset:0) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, 2, len(res.Query[0].Args))
	require.Equal(t, "1", res.Query[0].Args["first"])
	require.Equal(t, "0", res.Query[0].Args["offset"])
	require.Equal(t, childAttrs(res.Query[0]), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(res.Query[0].Children[0]), []string{"name"})
}

func TestParse(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, childAttrs(res.Query[0]), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(res.Query[0].Children[0]), []string{"name"})
}

func TestParseError(t *testing.T) {
	query := `
		me(func: uid(0x0a)) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid operation type: me")
}

func TestParseXid(t *testing.T) {
	query := `
	query {
		user(func: uid( 0x11)) {
			type.object.name
		}
	}`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name"})
}

func TestParseIdList(t *testing.T) {
	query := `
	query {
		user(func: uid(0x1)) {
			type.object.name
		}
	}`
	r, err := Parse(Request{Str: query, Http: true})
	gq := r.Query[0]
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, []string{"type.object.name"}, childAttrs(gq))
	//	require.Equal(t, []uint64{0x1}, gq.UID)
}

func TestParseIdList1(t *testing.T) {
	query := `
	query {
		user(func: uid(0x1, 0x34)) {
			type.object.name
		}
	}`
	r, err := Parse(Request{Str: query, Http: true})
	gq := r.Query[0]
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, []string{"type.object.name"}, childAttrs(gq))
	require.Equal(t, []uint64{0x1, 0x34}, gq.UID)
	require.Equal(t, 2, len(gq.UID))
}

func TestParseIdListError(t *testing.T) {
	query := `
	query {
		user(func: uid( [0x1, 0x1, abc, ade, 0x34))] {
			type.object.name
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unexpected character [ while parsing request")
}

func TestParseIdListError2(t *testing.T) {
	query := `
	query {
		user(func: uid( [0x1, 0x1, 2, 3, 0x34])) {
			type.object.name
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unexpected character [ while parsing request.")
}

func TestParseFirst(t *testing.T) {
	query := `
	query {
		user(func: uid( 0x1)) {
			type.object.name
			friends (first: 10) {
			}
		}
	}`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"type.object.name", "friends"}, childAttrs(res.Query[0]))
	require.Equal(t, "10", res.Query[0].Children[1].Args["first"])
}

func TestParseFirst_error(t *testing.T) {
	query := `
	query {
		user(func: uid( 0x1)) {
			type.object.name
			friends (first: ) {
			}
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expecting argument value")
	require.Contains(t, err.Error(), "\")\"")
}

func TestParseAfter(t *testing.T) {
	query := `
	query {
		user(func: uid( 0x1)) {
			type.object.name
			friends (first: 10, after: 3) {
			}
		}
	}`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Args["first"], "10")
	require.Equal(t, res.Query[0].Children[1].Args["after"], "3")
}

func TestParseOffset(t *testing.T) {
	query := `
	query {
		user(func: uid( 0x1)) {
			type.object.name
			friends (first: 10, offset: 3) {
			}
		}
	}`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Args["first"], "10")
	require.Equal(t, res.Query[0].Children[1].Args["offset"], "3")
}

func TestParseOffset_error(t *testing.T) {
	query := `
	query {
		user(func: uid( 0x1)) {
			type.object.name
			friends (first: 10, offset: ) {
			}
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expecting argument value")
	require.Contains(t, err.Error(), "\")\"")
}

func TestParse_error2(t *testing.T) {
	query := `
		query {
			me {
				name
			}
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected Left round brackets")
	require.Contains(t, err.Error(), "\"{\"")

}

func TestParse_pass1(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name,
				friends(xid:what) {  # xid would be ignored.
				}
			}
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got invalid keyword: xid")
}

func TestParse_alias_count(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name,
				bestFriend: friends(first: 10) {
					nameCount: count(name)
				}
			}
		}
	`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Alias, "bestFriend")
	require.Equal(t, childAttrs(res.Query[0].Children[1]), []string{"name"})
	require.Equal(t, "nameCount", res.Query[0].Children[1].Children[0].Alias)
}

func TestParse_alias_var(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name,
				f as bestFriend: friends(first: 10) {
					c as count(friend)
				}
			}

			friend(func: uid(f)) {
				name
				fcount: val(c)
			}
		}
	`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Alias, "bestFriend")
	require.Equal(t, "fcount", res.Query[1].Children[1].Alias)
}

func TestParse_alias_max(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name,
				bestFriend: friends(first: 10) {
					x as count(friends)
				}
				maxfriendcount: max(val(x))
			}
		}
	`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, res.Query[0].Children[1].Alias, "bestFriend")
	require.Equal(t, "maxfriendcount", res.Query[0].Children[2].Alias)
}

func TestParse_alias(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name,
				bestFriend: friends(first: 10) {
					name
				}
			}
		}
	`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Alias, "bestFriend")
	require.Equal(t, childAttrs(res.Query[0].Children[1]), []string{"name"})
}

func TestParse_alias1(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name: type.object.name.en
				bestFriend: friends(first: 10) {
					name: type.object.name.hi
				}
			}
		}
	`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name.en", "friends"})
	require.Equal(t, res.Query[0].Children[1].Alias, "bestFriend")
	require.Equal(t, res.Query[0].Children[1].Children[0].Alias, "name")
	require.Equal(t, childAttrs(res.Query[0].Children[1]), []string{"type.object.name.hi"})
}

func TestParse_block(t *testing.T) {
	query := `
		{
			root(func: uid( 0x0a)) {
				type.object.name.es.419
			}
		}
	`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name.es.419"})
}

func TestParseSchema(t *testing.T) {
	query := `
		schema (pred : name) {
			pred
			type
		}
	`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, res.Schema.Predicates[0], "name")
	require.Equal(t, len(res.Schema.Fields), 2)
	require.Equal(t, res.Schema.Fields[0], "pred")
	require.Equal(t, res.Schema.Fields[1], "type")
}

func TestParseSchemaMulti(t *testing.T) {
	query := `
		schema (pred : [name,hi]) {
			pred
			type
		}
	`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, len(res.Schema.Predicates), 2)
	require.Equal(t, res.Schema.Predicates[0], "name")
	require.Equal(t, res.Schema.Predicates[1], "hi")
	require.Equal(t, len(res.Schema.Fields), 2)
	require.Equal(t, res.Schema.Fields[0], "pred")
	require.Equal(t, res.Schema.Fields[1], "type")
}

func TestParseSchemaAll(t *testing.T) {
	query := `
		schema {
			pred
			type
		}
	`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, len(res.Schema.Predicates), 0)
	require.Equal(t, len(res.Schema.Fields), 2)
	require.Equal(t, res.Schema.Fields[0], "pred")
	require.Equal(t, res.Schema.Fields[1], "type")
}

func TestParseSchemaWithComments(t *testing.T) {
	query := `
		schema (pred : name) {
			#hi
			pred #bye
			type
		}
	`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, res.Schema.Predicates[0], "name")
	require.Equal(t, len(res.Schema.Fields), 2)
	require.Equal(t, res.Schema.Fields[0], "pred")
	require.Equal(t, res.Schema.Fields[1], "type")
}

func TestParseSchemaAndQuery(t *testing.T) {
	query1 := `
		schema {
			pred
			type
		}
		query {
			me(func: uid( tomhanks)) {
				name
				hometown
			}
		}
	`
	query2 := `
		query {
			me(func: uid( tomhanks)) {
				name
				hometown
			}
		}
		schema {
			pred
			type
		}
	`

	_, err := Parse(Request{Str: query1, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "schema block is not allowed with query block")

	_, err = Parse(Request{Str: query2, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "schema block is not allowed with query block")
}

func TestParseSchemaError(t *testing.T) {
	query := `
		schema () {
			pred
			type
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid schema block")
}

func TestParseSchemaErrorMulti(t *testing.T) {
	query := `
		schema {
			pred
			type
		}
		schema {
			pred
			type
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only one schema block allowed")
}

func TestParseMutationError(t *testing.T) {
	query := `
		mutation {
			set {
				<name> <is> <something> .
				<hometown> <is> <san/francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
}

func TestParseMutationAndQueryWithComments(t *testing.T) {
	query := `
	# Mutation
		mutation {
			# Set block
			set {
				<name> <is> <something> .
				<hometown> <is> <san/francisco> .
			}
			# Delete block
			delete {
				<name> <is> <something-else> .
			}
		}
		# Query starts here.
		query {
			me(func: uid( 0x5)) { # now mention children
				name		# Name
				hometown # hometown of the person
			}
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
}

func TestParseFragmentMultiQuery(t *testing.T) {
	query := `
	{
		user(func: uid(0x0a)) {
			...fragmenta,...fragmentb
			friends {
				name
			}
			...fragmentc
			hobbies
			...fragmentd
		}

		me(func: uid(0x01)) {
			...fragmenta
			...fragmentb
		}
	}

	fragment fragmenta {
		name
	}

	fragment fragmentb {
		id
	}

	fragment fragmentc {
		name
	}

	fragment fragmentd {
		id
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"name", "id", "friends", "name", "hobbies", "id"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name", "id"}, childAttrs(res.Query[1]))
}

func TestParseFragmentNoNesting(t *testing.T) {
	query := `
	query {
		user(func: uid(0x0a)) {
			...fragmenta,...fragmentb
			friends {
				name
			}
			...fragmentc
			hobbies
			...fragmentd
		}
	}

	fragment fragmenta {
		name
	}

	fragment fragmentb {
		id
	}

	fragment fragmentc {
		name
	}

	fragment fragmentd {
		id
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "id", "friends", "name", "hobbies", "id"})
}

func TestParseFragmentNest1(t *testing.T) {
	query := `
	query {
		user(func: uid(0x0a)) {
			...fragmenta
			friends {
				name
			}
		}
	}

	fragment fragmenta {
		id
		...fragmentb
	}

	fragment fragmentb {
		hobbies
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"id", "hobbies", "friends"})
}

func TestParseFragmentNest2(t *testing.T) {
	query := `
	query {
		user(func: uid(0x0a)) {
			friends {
				...fragmenta
			}
		}
	}
	fragment fragmenta {
		name
		...fragmentb
	}
	fragment fragmentb {
		nickname
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"friends"})
	require.Equal(t, childAttrs(res.Query[0].Children[0]), []string{"name", "nickname"})
}

func TestParseFragmentCycle(t *testing.T) {
	query := `
	query {
		user(func: uid(0x0a)) {
			...fragmenta
		}
	}
	fragment fragmenta {
		name
		...fragmentb
	}
	fragment fragmentb {
		...fragmentc
	}
	fragment fragmentc {
		id
		...fragmenta
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err, "Expected error with cycle")
	require.Contains(t, err.Error(), "Cycle detected")
}

func TestParseFragmentMissing(t *testing.T) {
	query := `
	query {
		user(func: uid(0x0a)) {
			...fragmenta
		}
	}
	fragment fragmentb {
		...fragmentc
	}
	fragment fragmentc {
		id
		...fragmenta
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err, "Expected error with missing fragment")
	require.Contains(t, err.Error(), "Missing fragment: fragmenta")
}

func TestParseVarInFunc(t *testing.T) {
	query := `{
		"query" : "query versions($version: int!){versions(func:eq(type, $version)){versions{ version_number}}}",
		"variables" : {"$version": "3"}
	}`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, "3", res.Query[0].Func.Args[0].Value)
	require.Equal(t, false, res.Query[0].Func.Args[0].IsValueVar)
}

func TestParseStringVarInFilter(t *testing.T) {
	query := `
		query versions($version: string = "v0.7.3/beta")
		{
			versions(func:eq(type, "version"))
			{
				versions @filter(eq(version_number, $version))
				{
					version_number
				}
			}
		}
	`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, "v0.7.3/beta", res.Query[0].Children[0].Filter.Func.Args[0].Value)
}

func TestParseVarInFilter(t *testing.T) {
	query := `{
		"query" : "query versions($version: int!){versions(func:eq(type, \"version\")){versions @filter(eq(version_number, $version)) { version_number}}}",
		"variables" : {"$version": "3"}
	}`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, "3", res.Query[0].Children[0].Filter.Func.Args[0].Value)
}

func TestParseVariables(t *testing.T) {
	query := `{
		"query": "query testQuery( $a  : int   , $b: int){root(func: uid(0x0a)) {name(first: $b, after: $a){english}}}",
		"variables": {"$a": "6", "$b": "5" }
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestParseVariables1(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int , $b: int!){root(func: uid( 0x0a)) {name(first: $b){english}}}",
		"variables": {"$b": "5" }
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestParseVariables2(t *testing.T) {
	query := `{
		"query": "query testQuery($a: float , $b: bool!){root(func: uid( 0x0a)) {name{english}}}",
		"variables": {"$b": "false", "$a": "3.33" }
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestParseVariables3(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int , $b: int! ){root(func: uid( 0x0a)) {name(first: $b){english}}}",
		"variables": {"$a": "5", "$b": "3"}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestParseVariablesStringfiedJSON(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int! , $b: int){root(func: uid( 0x0a)) {name(first: $b){english}}}",
		"variables": "{\"$a\": \"5\" }"
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestParseVariablesDefault1(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int = 3  , $b: int =  4 ,  $c : int = 3){root(func: uid( 0x0a)) {name(first: $b, after: $a, offset: $c){english}}}",
		"variables": {"$b": "5" }
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}
func TestParseVariablesFragments(t *testing.T) {
	query := `{
	"query": "query test($a: int){user(func: uid(0x0a)) {...fragmentd,friends(first: $a, offset: $a) {name}}} fragment fragmentd {id(first: $a)}",
	"variables": {"$a": "5"}
}`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"id", "friends"})
	require.Empty(t, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, childAttrs(res.Query[0].Children[1]), []string{"name"})
	require.Equal(t, "5", res.Query[0].Children[0].Args["first"])
}

func TestParseVariablesNewLineInDefault(t *testing.T) {
	query := `{
		"query": "query test($a: string = \"Line1\\nLine2\") { q(func: eq(name, $a)) { name } }"
	}`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, "Line1\nLine2", res.Query[0].Func.Args[0].Value)
}

func TestParseVariablesNewLineInVarBlock(t *testing.T) {
	query := `{
		"query": "query test($a: string) { q(func: eq(name, $a)) { name } }",
		"variables": {"$a": "Line1\nLine2" }
	}`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, "Line1\nLine2", res.Query[0].Func.Args[0].Value)
}

func TestParseVariablesError1(t *testing.T) {
	query := `
	query testQuery($a: string, $b: int!){
			root(func: uid( 0x0a)) {
				type.object.name.es-419
			}
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Variable $")
	require.Contains(t, err.Error(), "should be initialised")
}

func TestParseVariablesError2(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: int, $c: int!){` +
		`   root(func: uid( 0x0a) {name(first: $b, after: $a)){english}}` +
		`}",
		"variables": {"$a": "6", "$b": "5" }
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err, "Expected value for variable $c")
	require.Contains(t, err.Error(), "Variable $c should be initialised")
}

func TestParseVariablesError3(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: , $c: int!){` +
		`   root(func: uid( 0x0a) {name(first: $b, after: $a)){english}}` +
		`}",
		"variables": {"$a": "6", "$b": "5" }
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err, "Expected type for variable $b")
}

func TestParseVariablesError4(t *testing.T) {
	query := `{
		"query": "query testQuery($a: bool , $b: float! = 3){root(func: uid( 0x0a) {name(first: $b)){english}}}",
		"variables": {"$a": "5" }
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err, "Expected type error")
	require.Contains(t, err.Error(), "Type ending with ! can't have default value")
}

func TestParseVariablesError5(t *testing.T) {
	query := `{
		"query": "query ($a: int, $b: int){root(func: uid( 0x0a) {name(first: $b, after: $a)){english}}}",
		"variables": {"$a": "6", "$b": "5" }
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err, "Expected error: Query with variables should be named")
	require.Contains(t, err.Error(), "Variables can be defined only in named queries")
}

func TestParseVariablesError6(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: random){root(func: uid( 0x0a) {name(first: $b, after: $a)){english}}}",
		"variables": {"$a": "6", "$b": "5" }
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err, "Expected error: Type random not supported")
	require.Contains(t, err.Error(), "Type random not supported")
}

func TestParseVariablesError7(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: int, $c: int!){` +
		`    root(func: uid( 0x0a) {name(first: $b, after: $a)){english}}` +
		`}",
		"variables": {"$a": "6", "$b": "5", "$c": "321", "$d": "abc" }
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err, "Expected type for variable $d")
	require.Contains(t, err.Error(), "Type of variable $d not specified")
}

func TestParseVariablesiError8(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int = 3  , $b: int! =  4 ,  $c : int = 3){root(func: uid( 0x0a) {name(first: $b, after: $a, offset: $c)){english}}}",
		"variables": {"$b": "5" }
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err, "Variables type ending with ! can't have default value")
	require.Contains(t, err.Error(), "Type ending with ! can't have default value")
}

func TestParseFilter_root(t *testing.T) {
	query := `
	query {
		me(func:anyofterms(abc, "Abc")) @filter(allofterms(name, "alice")) {
			friends @filter() {
				name @filter(namefilter(name, "a"))
			}
			gender @filter(eq(g, "a")),age @filter(neq(a, "b"))
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.NotNil(t, res.Query[0].Filter)
	require.Equal(t, `(allofterms name "alice")`, res.Query[0].Filter.debugString())
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Nil(t, res.Query[0].Children[0].Filter)
	require.Equal(t, `(eq g "a")`, res.Query[0].Children[1].Filter.debugString())
	require.Equal(t, `(neq a "b")`, res.Query[0].Children[2].Filter.debugString())
	require.Equal(t, `(namefilter name "a")`, res.Query[0].Children[0].Children[0].Filter.debugString())
}

func TestParseFuncNested(t *testing.T) {
	query := `
	query {
		me(func: gt(count(friend), 10)) {
			friends @filter() {
				name
			}
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.NotNil(t, res.Query[0].Func)
	require.Equal(t, res.Query[0].Func.Name, "gt")
	require.Equal(t, res.Query[0].Func.Args[0].Value, "10")
	require.Equal(t, res.Query[0].Func.IsCount, true)
}

func TestParseFuncNested2(t *testing.T) {
	query := `
	query {
		var(func:uid(1)) {
			a as name
		}
		me(func: eq(name, val(a))) {
			friends @filter() {
				name
			}
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[1])
	require.NotNil(t, res.Query[1].Func)
	require.Equal(t, res.Query[1].Func.Name, "eq")
	require.Equal(t, res.Query[1].Func.Args[0].Value, "a")
	require.Equal(t, res.Query[1].Func.Args[0].IsValueVar, true)
	require.Equal(t, res.Query[1].Func.IsCount, false)
}

func TestParseFilter_root2(t *testing.T) {
	query := `
	query {
		me(func:anyofterms(abc, "Abc")) @filter(gt(count(friends), 10)) {
			friends @filter() {
				name
			}
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.NotNil(t, res.Query[0].Filter)
	require.Equal(t, `(gt count(friends) "10")`, res.Query[0].Filter.debugString())
	require.Equal(t, []string{"friends", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Nil(t, res.Query[0].Children[0].Filter)
}

func TestParseFilter_root_Error2(t *testing.T) {
	// filter-by-count only support first argument as function
	query := `
	query {
		me(func:anyofterms(abc, "Abc")) @filter(gt(count(friends), sum(friends))) {
			friends @filter() {
				name
			}
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Multiple functions as arguments not allowed")
}

func TestParseFilter_simplest(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter() {
				name @filter(namefilter(name, "a"))
			}
			gender @filter(eq(g, "a")),age @filter(neq(a, "b"))
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Nil(t, res.Query[0].Children[0].Filter)
	require.Equal(t, `(eq g "a")`, res.Query[0].Children[1].Filter.debugString())
	require.Equal(t, `(neq a "b")`, res.Query[0].Children[2].Filter.debugString())
	require.Equal(t, `(namefilter name "a")`, res.Query[0].Children[0].Children[0].Filter.debugString())
}

// Test operator precedence. and should be evaluated before or.
func TestParseFilter_op(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(a(aa, "aaa") or b(bb, "bbb")
			and c(cc, "ccc")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, `(OR (a aa "aaa") (AND (b bb "bbb") (c cc "ccc")))`, res.Query[0].Children[0].Filter.debugString())
}

func TestParseFilter_opError1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(a(aa "aaa") or b(b "bbb")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected comma or language but got: \"aaa\"")
}

func TestParseFilter_opNoError2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(a(aa, "aaa") or b(b, "bbb")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	// It's valid.  Makes sure TestParseFilter_opError3 fails for the expected reason.
}

func TestParseFilter_opError3(t *testing.T) {
	query := `
		query {
		me(func: uid(0x0a)) {
			friends @filter(a(aa, "aaa") or b(b, "bbb") and) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid filter statement")
}

func TestParseFilter_opNot1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(not a(aa, "aaa")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, `(NOT (a aa "aaa"))`, res.Query[0].Children[0].Filter.debugString())
}

func TestParseFilter_opNot2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(not(a(aa, "aaa") or (b(bb, "bbb"))) and c(cc, "ccc")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, `(AND (NOT (OR (a aa "aaa") (b bb "bbb"))) (c cc "ccc"))`, res.Query[0].Children[0].Filter.debugString())
}

// Test operator precedence. Let brackets make or evaluates before and.
func TestParseFilter_op2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter((a(aa, "aaa") Or b(bb, "bbb"))
			 and c(cc, "ccc")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, `(AND (OR (a aa "aaa") (b bb "bbb")) (c cc "ccc"))`, res.Query[0].Children[0].Filter.debugString())
}

// Test operator precedence. More elaborate brackets.
func TestParseFilter_brac(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(  a(name, "hello") or b(name, "world", "is") and (c(aa, "aaa") or (d(dd, "haha") or e(ee, "aaa"))) and f(ff, "aaa")){
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t,
		`(OR (a name "hello") (AND (AND (b name "world" "is") (OR (c aa "aaa") (OR (d dd "haha") (e ee "aaa")))) (f ff "aaa")))`,
		res.Query[0].Children[0].Filter.debugString())
}

// Test if unbalanced brac will lead to errors.
func TestParseFilter_unbalancedbrac(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(  () {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unexpected item while parsing @filter")
	require.Contains(t, err.Error(), "'{'")
}

func TestParseFilter_Geo1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(near(loc, [-1.12 , 2.0123 ], 100.123 )) {
				name
			}
			gender,age
			hometown
		}
	}
`
	resp, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, "[-1.12,2.0123]", resp.Query[0].Children[0].Filter.Func.Args[0].Value)
	require.Equal(t, "100.123", resp.Query[0].Children[0].Filter.Func.Args[1].Value)
	require.Equal(t, false, resp.Query[0].Children[0].Filter.Func.Args[0].IsValueVar)
	require.Equal(t, false, resp.Query[0].Children[0].Filter.Func.Args[1].IsValueVar)
}

func TestParseFilter_Geo2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(within(loc, [[11.2 , -2.234 ], [ -31.23, 4.3214] , [5.312, 6.53]] )) {
				name
			}
			gender,age
			hometown
		}
	}
`
	resp, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, "[[11.2,-2.234],[-31.23,4.3214],[5.312,6.53]]", resp.Query[0].Children[0].Filter.Func.Args[0].Value)
}

func TestParseFilter_Geo3(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(near(loc, [[1 , 2 ], [[3, 4] , [5, 6]] )) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Found invalid item")
}

func TestParseFilter_Geo4(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(near(loc, [[1 , 2 ], [3, 4] , [5, 6]]] )) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected right round or comma")
	require.Contains(t, err.Error(), "\"]\"")
}

// Test if empty brackets will lead to errors.
func TestParseFilter_emptyargument(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(allofterms(name,,)) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Consecutive commas not allowed")

}

func TestParseFilter_unknowndirectiveError1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filtererr {
				name
			}
			gender,age
			hometown
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	// We can't differentiate between @filtererr being a directive or a language. As we don't
	// see a () after it we assume its a language but attr which specify a language can't have
	// children.
	// The test below tests for unknown directive.
	require.Contains(t, err.Error(), "Cannot have children for attr: friends with lang tags:")
}

func TestParseFilter_unknowndirectiveError2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filtererr ()
			gender,age
			hometown
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unknown directive [filtererr]")
}

func TestParseGeneratorError1(t *testing.T) {
	query := `{
		me(allofterms(name, "barack")) {
			friends {
				name
			}
			gender,age
			hometown
			count(friends)
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got invalid keyword: allofterms")
}

func TestParseGeneratorError2(t *testing.T) {
	query := `{
		me(func: allofterms(name, "barack")) {
			friends(all: 5) {
				name
			}
			gender,age
			hometown
			count(friends)
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got invalid keyword: all")
}

func TestParseQuotedFunctionAttributeError(t *testing.T) {
	query := `{
		me(func: allofterms("name", "barack")) {
			friends {
				name
			}
			gender,age
			hometown
			count(friends)
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Attribute in function must not be quoted")
}

func TestParseCountAsFuncMultiple(t *testing.T) {
	query := `{
		me(func: uid(1)) {
			count(friends), count(relatives)
			count(classmates)
			gender,age
			hometown
		}
	}
`
	gq, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, 6, len(gq.Query[0].Children))
	require.Equal(t, true, gq.Query[0].Children[0].IsCount)
	require.Equal(t, "friends", gq.Query[0].Children[0].Attr)
	require.Equal(t, true, gq.Query[0].Children[1].IsCount)
	require.Equal(t, "relatives", gq.Query[0].Children[1].Attr)
	require.Equal(t, true, gq.Query[0].Children[2].IsCount)
	require.Equal(t, "classmates", gq.Query[0].Children[2].Attr)
}

func TestParseCountAsFuncMultipleError(t *testing.T) {
	query := `{
		me(func: uid(1)) {
			count(friends, relatives
			classmates)
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Multiple predicates not allowed in single count")
}

func TestParseCountAsFunc(t *testing.T) {
	query := `{
		me(func: uid(1)) {
			count(friends)
			gender,age
			hometown
		}
	}
`
	gq, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, true, gq.Query[0].Children[0].IsCount)
	require.Equal(t, 4, len(gq.Query[0].Children))

}

func TestParseCountError1(t *testing.T) {
	query := `{
		me(func: uid(1)) {
			count(friends
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Multiple predicates not allowed in single count")
}

func TestParseCountError2(t *testing.T) {
	query := `{
		me(func: uid(1)) {
			count((friends)
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Predicate name cannot be empty")
}

func TestParseCheckPwd(t *testing.T) {

	query := `{
		me(func: uid(1)) {
			checkpwd(password, "123456")
			hometown
		}
	}
`
	gq, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, "checkpwd", gq.Query[0].Children[0].Func.Name)
	require.Equal(t, "123456", gq.Query[0].Children[0].Func.Args[0].Value)
	require.Equal(t, "password", gq.Query[0].Children[0].Attr)
}

func TestParseComments(t *testing.T) {
	query := `
	# Something
	{
		me(func:allofterms(name, "barack")) {
			friends {
				name
			} # Something
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestParseComments1(t *testing.T) {
	query := `{
		#Something
		me(func:allofterms(name, "barack")) {
			friends {
				name  # Name of my friend
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestParseGenerator(t *testing.T) {
	query := `{
		me(func:allofterms(name, "barack")) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestParseIRIRef(t *testing.T) {
	query := `{
		me(func: uid( 0x1)) {
			<http://verygood.com/what/about/you>
			friends @filter(allofterms(<http://verygood.com/what/about/you>,
				"good better bad")){
				name
			}
			gender,age
			hometown
		}
	}`

	gq, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, 5, len(gq.Query[0].Children))
	require.Equal(t, "http://verygood.com/what/about/you", gq.Query[0].Children[0].Attr)
	require.Equal(t, `(allofterms http://verygood.com/what/about/you "good better bad")`,
		gq.Query[0].Children[1].Filter.debugString())
}

func TestParseIRIRef2(t *testing.T) {
	query := `{
		me(func:anyofterms(<http://helloworld.com/how/are/you>, "good better bad")) {
			<http://verygood.com/what/about/you>
			friends @filter(allofterms(<http://verygood.com/what/about/you>,
				"good better bad")){
				name
			}
		}
	}`

	gq, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, 2, len(gq.Query[0].Children))
	require.Equal(t, "http://verygood.com/what/about/you", gq.Query[0].Children[0].Attr)
	require.Equal(t, `(allofterms http://verygood.com/what/about/you "good better bad")`,
		gq.Query[0].Children[1].Filter.debugString())
	require.Equal(t, "http://helloworld.com/how/are/you", gq.Query[0].Func.Attr)
}

func TestParseIRIRefSpace(t *testing.T) {
	query := `{
		me(func: uid( <http://helloworld.com/how/are/ you>)) {
		}
	      }`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err) // because of space.
	require.Contains(t, err.Error(), "Unexpected character ' ' while parsing IRI")
}

func TestParseIRIRefInvalidChar(t *testing.T) {
	query := `{
		me(func: uid( <http://helloworld.com/how/are/^you>)) {
		}
	      }`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err) // because of ^
	require.Contains(t, err.Error(), "Unexpected character '^' while parsing IRI")
}

func TestLangs(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@en,name@en:ru:hu
		}
	}
	`

	gq, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, 2, len(gq.Query[0].Children))
	require.Equal(t, "name", gq.Query[0].Children[0].Attr)
	require.Equal(t, []string{"en"}, gq.Query[0].Children[0].Langs)
	require.Equal(t, "name", gq.Query[0].Children[1].Attr)
	require.Equal(t, []string{"en", "ru", "hu"}, gq.Query[0].Children[1].Langs)
}

func TestLangsInvalid1(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@en@ru
		}
	}
	`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected directive or language list, got @ru")
}

func TestLangsInvalid2(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			@en:ru
		}
	}
	`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid use of directive.")
}

func TestLangsInvalid3(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@en:ru, @en:ru
		}
	}
	`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected directive or language list, got @en")
}

func TestLangsInvalid4(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@
		}
	}
	`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected directive or language list")
}

func TestLangsInvalid5(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@<something.wrong>
		}
	}
	`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected directive or language list")
}

func TestLangsInvalid6(t *testing.T) {
	query := `
		{
			me(func: uid(0x1004)) {
				name@hi:cn:...
			}
		}
	`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected only one dot(.) while parsing language list.")
}

func TestLangsInvalid7(t *testing.T) {
	query := `
		{
			me(func: uid(0x1004)) {
				name@...
			}
		}
	`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected only one dot(.) while parsing language list.")
}

func TestLangsFilter(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(alloftext(descr@en, "something")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.NotNil(t, res.Query[0].Children[0])
	require.NotNil(t, res.Query[0].Children[0].Filter)
	require.NotNil(t, res.Query[0].Children[0].Filter.Func)
	require.Equal(t, "descr", res.Query[0].Children[0].Filter.Func.Attr)
	require.Equal(t, "en", res.Query[0].Children[0].Filter.Func.Lang)
}

func TestLangsFilter_error1(t *testing.T) {
	// this query should fail, because '@lang' is used twice (and only one appearance is allowed)
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(alloftext(descr@en@de, "something")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid usage of '@' in function argument")
}

func TestLangsFilter_error2(t *testing.T) {
	// this query should fail, because there is no lang after '@'
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(alloftext(descr@, "something")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected arg after func [alloftext]")
	require.Contains(t, err.Error(), "','")
}

func TestLangsFunction(t *testing.T) {
	query := `
	query {
		me(func:alloftext(descr@en, "something")) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.NotNil(t, res.Query[0].Func)
	require.Equal(t, "descr", res.Query[0].Func.Attr)
	require.Equal(t, "en", res.Query[0].Func.Lang)
}

func TestLangsFunctionMultipleLangs(t *testing.T) {
	query := `
	query {
		me(func:alloftext(descr@hi:en, "something")) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected arg after func [alloftext]")
	require.Contains(t, err.Error(), "\":\"")
}

func TestParseNormalize(t *testing.T) {
	query := `
	query {
		me(func: uid( 0x3)) @normalize {
			friends {
				name
			}
			gender
			hometown
		}
}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.True(t, res.Query[0].Normalize)
}

func TestParseGroupbyRoot(t *testing.T) {
	query := `
	query {
		me(func: uid(1, 2, 3)) @groupby(friends) {
				a as count(uid)
		}

		groups(func: uid(a)) {
			uid
			val(a)
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, 1, len(res.Query[0].GroupbyAttrs))
	require.Equal(t, "friends", res.Query[0].GroupbyAttrs[0].Attr)
	require.Equal(t, "a", res.Query[0].Children[0].Var)
}

func TestParseGroupbyWithCountVar(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @groupby(friends) {
				a as count(uid)
			}
			hometown
			age
		}

		groups(func: uid(a)) {
			uid
			val(a)
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, 1, len(res.Query[0].Children[0].GroupbyAttrs))
	require.Equal(t, "friends", res.Query[0].Children[0].GroupbyAttrs[0].Attr)
	require.Equal(t, "a", res.Query[0].Children[0].Children[0].Var)
}

func TestParseGroupbyWithMaxVar(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @groupby(friends) {
				a as max(first-name@en:ta)
			}
			hometown
			age
		}

		groups(func: uid(a)) {
			uid
			val(a)
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, 1, len(res.Query[0].Children[0].GroupbyAttrs))
	require.Equal(t, "friends", res.Query[0].Children[0].GroupbyAttrs[0].Attr)
	require.Equal(t, "first-name", res.Query[0].Children[0].Children[0].Attr)
	require.Equal(t, []string{"en", "ta"}, res.Query[0].Children[0].Children[0].Langs)
	require.Equal(t, "a", res.Query[0].Children[0].Children[0].Var)
}

func TestParseGroupby(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @groupby(name@en) {
				count(uid)
			}
			hometown
			age
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, 1, len(res.Query[0].Children[0].GroupbyAttrs))
	require.Equal(t, "name", res.Query[0].Children[0].GroupbyAttrs[0].Attr)
	require.Equal(t, "en", res.Query[0].Children[0].GroupbyAttrs[0].Langs[0])
}

func TestParseGroupbyError(t *testing.T) {
	// predicates not allowed inside groupby.
	query := `
	query {
		me(func: uid(0x1)) {
			friends @groupby(name) {
				name
				count(uid)
			}
			hometown
			age
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only aggregator/count functions allowed inside @groupby")
}

func TestParseFacetsError1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(facet1,, facet2)
			}
			hometown
			age
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected ( after func name [facet1]")
}

func TestParseFacetsVarError(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(facet1, b as)
			}
			hometown
			age
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected name in facet list")
}
func TestParseFacetsError2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(facet1 facet2)
			}
			hometown
			age
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected ( after func name [facet1]")
}

func TestParseFacetsOrderError1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(orderdesc: orderdesc: closeness) {
				name
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected ( after func name [orderdesc]")
}

func TestParseFacetsOrderError2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(a as b as closeness) {
				name
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected ( after func name [a]")
}

func TestParseFacetsOrderError3(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(orderdesc: closeness, order: abc) {
				name
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected ',' or ')'")
}

func TestParseFacetsDuplicateVarError(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(a as closeness, b as closeness) {
				name
			}
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Duplicate variable mappings")
}

func TestParseFacetsOrderVar(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(orderdesc: a as b) {
				name
			}
		}
		me(func: uid(a)) { }
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestParseFacetsOrderVar2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(a as orderdesc: b) {
				name
			}
		}
		me(func: uid(a)) {

		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected ( after func name [a]")
}

func TestParseFacets(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(orderdesc: closeness) {
				name
			}
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, "closeness", res.Query[0].Children[0].FacetOrder)
	require.True(t, res.Query[0].Children[0].FacetDesc)
}

func TestParseOrderbyFacet(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(facet1)
			}
			hometown
			age
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "hometown", "age"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, true, res.Query[0].Children[0].Facets.AllKeys)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.NotNil(t, res.Query[0].Children[0].Children[0].Facets)
	require.Equal(t, false, res.Query[0].Children[0].Children[0].Facets.AllKeys)
	require.Equal(t, "facet1", res.Query[0].Children[0].Children[0].Facets.Keys[0])
}

func TestParseFacetsMultiple(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(key1, key2, key3)
			}
			hometown
			age
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "hometown", "age"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, true, res.Query[0].Children[0].Facets.AllKeys)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.NotNil(t, res.Query[0].Children[0].Children[0].Facets)
	require.Equal(t, false, res.Query[0].Children[0].Children[0].Facets.AllKeys)
	require.Equal(t, 3, len(res.Query[0].Children[0].Children[0].Facets.Keys))
}

func TestParseFacetsMultipleVar(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(a as key1, key2, b as key3)
			}
			hometown
			age
		}
		h(func: uid(a, b)) {
			uid
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "hometown", "age"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, true, res.Query[0].Children[0].Facets.AllKeys)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.NotNil(t, res.Query[0].Children[0].Children[0].Facets)
	require.Equal(t, false, res.Query[0].Children[0].Children[0].Facets.AllKeys)
	require.Equal(t, 3, len(res.Query[0].Children[0].Children[0].Facets.Keys))
	require.Equal(t, "a", res.Query[0].Children[0].Children[0].FacetVar["key1"])
	require.Equal(t, "", res.Query[0].Children[0].Children[0].FacetVar["key2"])
	require.Equal(t, "b", res.Query[0].Children[0].Children[0].FacetVar["key3"])
}

func TestParseFacetsMultipleRepeat(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(key1, key2, key3, key1)
			}
			hometown
			age
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "hometown", "age"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, true, res.Query[0].Children[0].Facets.AllKeys)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.NotNil(t, res.Query[0].Children[0].Children[0].Facets)
	require.Equal(t, false, res.Query[0].Children[0].Children[0].Facets.AllKeys)
	require.Equal(t, 3, len(res.Query[0].Children[0].Children[0].Facets.Keys))
}

func TestParseFacetsEmpty(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets() {
			}
			hometown
			age
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "hometown", "age"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, false, res.Query[0].Children[0].Facets.AllKeys)
	require.Equal(t, 0, len(res.Query[0].Children[0].Facets.Keys))
}

func TestParseFacetsFail1(t *testing.T) {
	// key can not be empty..
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(key1,, key2) {
			}
			hometown
			age
		}
	}
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected ( after func name [key1]")
}

func TestParseRepeatArgsError1(t *testing.T) {
	// key can not be empty..
	query := `
	{
  		me(func: anyoftext(Text, "biology"), func: anyoftext(Text, "science")) {
			Text
  		}
	}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only one function allowed at root")
}

func TestParseRepeatArgsError2(t *testing.T) {
	// key can not be empty..
	query := `
	{
  		me(func: anyoftext(Text, "science")) {
			Text(first: 1, first: 4)
  		}
	}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got repeated key \"first\"")
}

// Test facets parsing for filtering..
func TestFacetsFilterSimple(t *testing.T) {
	// all friends of 0x1 who are close to him
	query := `
	       {
		me(func: uid(0x1)) {
			name
			friend @facets(eq(close, true)) {
				name
				gender
			}
		}
	}
`

	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"name", "friend"}, childAttrs(res.Query[0]))
	require.Nil(t, res.Query[0].Children[1].Facets)
	require.NotNil(t, res.Query[0].Children[1].FacetsFilter)
	require.Equal(t, `(eq close "true")`,
		res.Query[0].Children[1].FacetsFilter.debugString())
}

func TestFacetsFilterAll(t *testing.T) {
	// all friends of 0x1 who are close to him or are in his family
	query := `
	       {
		me(func: uid(0x1)) {
			name
			friend @facets(eq(close, true) or eq(family, true)) @facets(close, family, since) {
				name @facets
				gender
			}
		}
	}
`

	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"name", "friend"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[1].Facets)
	require.Equal(t, "close", res.Query[0].Children[1].Facets.Keys[0])
	require.Equal(t, "family", res.Query[0].Children[1].Facets.Keys[1])
	require.Equal(t, "since", res.Query[0].Children[1].Facets.Keys[2])
	require.NotNil(t, res.Query[0].Children[1].FacetsFilter)
	require.Equal(t, `(OR (eq close "true") (eq family "true"))`,
		res.Query[0].Children[1].FacetsFilter.debugString())

	require.Equal(t, []string{"name", "gender"}, childAttrs(res.Query[0].Children[1]))
	nameChild := res.Query[0].Children[1].Children[0]
	require.NotNil(t, nameChild)
	require.NotNil(t, nameChild.Facets)
	require.Nil(t, nameChild.FacetsFilter)
	genderChild := res.Query[0].Children[1].Children[1]
	require.NotNil(t, genderChild)
	require.Nil(t, genderChild.Facets)
	require.Nil(t, genderChild.FacetsFilter)
}

func TestFacetsFilterFail(t *testing.T) {
	// multiple @facets and @facets(close, since) are not allowed.
	query := `
	       {
		me(func: uid(0x1)) {
			name
			friend @facets @facets(close, since) {
				name
				gender
			}
		}
	}
`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only one facets allowed")
}

func TestFacetsFilterFail2(t *testing.T) {
	// multiple facets-filter not allowed
	query := `
	       {
		me(func: uid(0x1)) {
			name
			friend @facets(eq(close, true)) @facets(eq(family, true)) {
				name
				gender
			}
		}
	}
`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only one facets filter allowed")
}

func TestFacetsFilterFail3(t *testing.T) {
	// vars are not allowed in facets filtering.
	query := `
	{
		K as var(func: uid(0x0a)) {
			L AS friends
		}
		me(func: uid(K)) {
			friend @facets(uid(L)) {
				name
			}
		}
	}
`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "variables are not allowed in facets filter")
}

func TestFacetsFilterFailRoot(t *testing.T) {
	query := `
	{
		me(func: uid(0x1)) @facets(eq(some-facet, true)) {
			friend	{
				name
			}
		}
	}
`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unknown directive [facets]")
}

func TestFacetsFilterAtValue(t *testing.T) {
	// gql parses facets at value level as well.
	query := `
	{
		me(func: uid(0x1)) {
			friend	{
				name @facets(eq(some.facet, true))
			}
		}
	}
`

	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	nameChild := res.Query[0].Children[0].Children[0]
	require.NotNil(t, nameChild)
	require.NotNil(t, nameChild.FacetsFilter)
	require.Equal(t, `(eq some.facet "true")`, nameChild.FacetsFilter.debugString())
}

func TestParseQueryWithAttrLang(t *testing.T) {
	query := `
	{
		me(func: uid(0x1)) {
			name
			friend(first:5, orderasc: name@en:fr) {
				name@en
			}
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, "name", res.Query[0].Children[1].Order[0].Attr)
	require.Equal(t, []string{"en", "fr"}, res.Query[0].Children[1].Order[0].Langs)
}

func TestParseQueryWithAttrLang2(t *testing.T) {
	query := `
	{
	  me(func:regexp(name, /^[a-zA-z]*[^Kk ]?[Nn]ight/), orderasc: name@en, first:5) {
		name@en
		name@de
		name@it
	  }
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, "name", res.Query[0].Order[0].Attr)
	require.Equal(t, []string{"en"}, res.Query[0].Order[0].Langs)
}

func TestParseRegexp1(t *testing.T) {
	query := `
	{
	  me(func: uid(0x1)) {
	    name
		friend @filter(regexp(name@en, /case INSENSITIVE regexp with \/ escaped value/i)) {
	      name@en
	    }
	  }
    }
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, "case INSENSITIVE regexp with / escaped value",
		res.Query[0].Children[1].Filter.Func.Args[0].Value)
	require.Equal(t, "i", res.Query[0].Children[1].Filter.Func.Args[1].Value)
}

func TestParseRegexp2(t *testing.T) {
	query := `
	{
	  me(func:regexp(name@en, /another\/compilicated ("") regexp('')/)) {
	    name
	  }
    }
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, "another/compilicated (\"\") regexp('')",
		res.Query[0].Func.Args[0].Value)
	require.Equal(t, "", res.Query[0].Func.Args[1].Value)
}

func TestParseRegexp3(t *testing.T) {
	query := `
	{
	  me(func:allofterms(name, "barack")) @filter(regexp(secret, /whitehouse[0-9]{1,4}/fLaGs)) {
	    name
	  }
    }
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, "whitehouse[0-9]{1,4}", res.Query[0].Filter.Func.Args[0].Value)
	require.Equal(t, "fLaGs", res.Query[0].Filter.Func.Args[1].Value)
}

func TestParseRegexp4(t *testing.T) {
	query := `
	{
	  me(func:regexp(name@en, /pattern/123)) {
	    name
	  }
    }
`
	_, err := Parse(Request{Str: query, Http: true})
	// only [a-zA-Z] characters can be used as flags
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected comma or language but got: 123")
}

func TestParseRegexp5(t *testing.T) {
	query := `
	{
	  me(func:regexp(name@en, /pattern/flag123)) {
	    name
	  }
    }
`
	_, err := Parse(Request{Str: query, Http: true})
	// only [a-zA-Z] characters can be used as flags
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected comma or language but got: 123")
}

func TestParseRegexp6(t *testing.T) {
	query := `
	{
	  me(func:regexp(name@en, /pattern\/)) {
	    name
	  }
    }
`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected arg after func [regexp]")
	require.Contains(t, err.Error(), "Unclosed regexp")
}

func TestParseGraphQLVarId(t *testing.T) {
	query := `{
		"query" : "query versions($a: string, $b: string, $c: string){versions(func: uid($a,$b,$c)){versions{ version_number}}}",
		"variables" : {"$a": "3", "$b": "3", "$c": "3"}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only one GraphQL variable allowed inside uid function.")
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestCountAtRoot(t *testing.T) {
	query := `{
		me(func: uid( 1)) {
			count(uid)
			count(enemy)
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestCountAtRootErr(t *testing.T) {
	query := `{
		me(func: uid( 1)) {
			count(enemy) {
				name
			}
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot have children attributes when asking for count")
}

func TestCountAtRootErr2(t *testing.T) {
	query := `{
		me(func: uid( 1)) {
			a as count(uid)
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot assign variable to count()")
}

func TestCountAtRootErr3(t *testing.T) {
	query := `{
		me(func: uid( 1)) {
			count()
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot use count(), please use count(uid)")
}

func TestHasFuncAtRoot(t *testing.T) {
	query := `{
		me(func: has(name@en)) {
			name
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestHasFilterAtRoot(t *testing.T) {
	query := `{
		me(func: allofterms(name, "Steven Tom")) @filter(has(director.film)) {
			name
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestHasFilterAtChild(t *testing.T) {
	query := `{
		me(func: anyofterms(name, "Steven Tom")) {
			name
			director.film @filter(has(genre)) {
			}
		}
	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

// this test tests parsing of EOF inside '...'
func TestDotsEOF(t *testing.T) {
	query := `{
		me(func: uid( 0x1)) {
			name
			..`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected 3 periods")
}

func TestMathWithoutVarAlias(t *testing.T) {
	query := `{
			f(func: anyofterms(name, "Rick Michonne Andrea")) {
				ageVar as age
				math(ageVar *2)
			}
		}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Function math should be used with a variable or have an alias")
}

func TestMultipleEqual(t *testing.T) {
	query := `{
		me(func: eq(name,["Steven Spielberg", "Tom Hanks"])) {
			name
		}
	}`

	gql, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, 2, len(gql.Query[0].Func.Args))
	require.Equal(t, "Tom Hanks", gql.Query[0].Func.Args[1].Value)
}

func TestParseEqArg(t *testing.T) {
	query := `
	{
		me(func: uid(1, 20)) @filter(eq(name, ["And\"rea", "Bob"])) {
		 name
		}
	}
`
	gql, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, 2, len(gql.Query[0].Filter.Func.Args))
}

// TestParserFuzz replays inputs that were identified by go-fuzz to cause crash
// in the past. Used for regression testing.
// We don't care here about return value, only about correct handling of
// incorrect input.
func TestParserFuzz(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"test001", "{e(){@filter(q(/"},
		{"test002", "{e(){@filter(p(%"},
		{"test003", "{e(){@filter(0(/\\0"},
		{"test004", "query #"},
		{"test005", "{e(){{@filter(q(/"},
		{"test006", "{e(func:0(0(0,/"},
		{"test007", "{e(func:uid()){@filter(0(/\\"},
		{"test008", "{e(){@filter(0(/\\0/"},
		{"test009", "{e(){@filter(p(/\\"},
		{"test010", "{m(func:0(0(0(0,/"},
		{"test011", "{e(){@filter(0(%0"},
		{"test012", "{e(func:uid()){@filter(p(%"},
		{"test013", "{e(orderasc:val(0)){{min(val(0)0("},
		{"test014", "query{e(func:uid(val(0)"},
		{"test015", "{e(){@filter(0(%000"},
		{"test016", "{e(){@filter(0(%"},
		{"test017", "{e(){{@filter(0(/"},
		{"test018", "{e(func:uid(0))@filter(p(/\\"},
		{"test019", "{e(func:uid()){@filter(p(/\\"},
		{"test020", "{e(){@filter(p(/\\00"},
		{"test021", "{e(func:uid(0)){@filter(0(%"},
		{"test022", "{e(){@filter(0(/"},
		{"test023", "{e(func:uid()){@filter(p(/"},
		{"test024", "{s(func:uid(val(0)"},
		{"test025", "{e()@filter(0(%"},
		{"test026", "{e(func:uid(0)){@filter(0(/\\"},
		{"test027", "{e(){@filter(0(%"},
		{"test028", "{e(){@filter(0(%00"},
		{"test029", "{e(func:uid(0)){@filter(p(/\\e/i)){e}}}"},
		{"test030", "{e(func:uid(0)){@filter(p(%//i))00"},
		{"test031", "{e()@filter(p(/�Is))}"},
		{"test032", "{e(){@filter(0(0,/"},
		{"test033", "{e(func:uid()){@filter(0(/"},
		{"test034", "{e()@filter(0(/"},
		{"test035", "{e(func:uid(0)){@filter(p(/\\"},
		{"test036", "{e(func:uid())@filter(0(%"},
		{"test037", "{e(func:uid(0)){@filter(0(%"},
		{"test038", "{e(func:uid(0))@filter(p(/\\0/"},
		{"test039", "{e(func:uid(0)){@filter(p(/\\e/i)){e}00"},
		{"test040", "{e(){@filter(0(-"},
		{"test041", "{e(func:uid(0)){@filter(0(%0"},
		{"test042", "{e()@filter(q(/"},
		{"test043", "{e(func:uid(0)){@filter(p(%"},
		{"test044", "{e()@filter(p(/"},
		{"test045", "{e(func:uid())@filter(0(/"},
		{"test046", "{e(func:uid(0)){@filter(p(/\\e/"},
		{"test047", "{e(func:uid()){@filter(0(%"},
		{"test048", "{e()@filter(0(0,/"},
		{"test049", "{e(){{@filter(0(0,/"},
		{"test050", "{e(func:uid(0)){@filter(p(/"},
		{"test051", "{e()@filter(0(-"},
		{"test052", "{e(func:uid(0)){@filter(0(/"},
		{"test053", "{e(func:uid())@filter(p(%"},
		{"test054", "{e(orderasc:val(0)){min(val(0)0("},
		{"test055", "{e(){@filter(p(/"},
		{"test056", "{e(func:uid())@filter(p(/"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("parser panic caused by test: '%s', input: '%s': %v\n%s", test.name, test.in, r, debug.Stack())
				}
			}()

			Parse(Request{Str: test.in, Http: true})
		})
	}
}

func TestParseEqArg2(t *testing.T) {
	query := `
	{
		me(func: eq(age, [1, 20])) @filter(eq(name, ["And\"rea", "Bob"])) {
		 name
		}
	}
`
	gql, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, 2, len(gql.Query[0].Filter.Func.Args))
	require.Equal(t, 2, len(gql.Query[0].Func.Args))
}

func TestFilterError(t *testing.T) {
	query := `
	{
		me(func: uid(1, 3 , 5, 7)) { @filter(uid(3, 7))
			name
		}
	}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
}

func TestFilterError2(t *testing.T) {
	query := `
	{
		me(func: uid(1, 3 , 5, 7)) {
			name @filter(eq(name, 	"abc")) @filter(eq(name2, 	"abc"))
		}
	}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
}

func TestDoubleGroupByError(t *testing.T) {
	query := `
	{
		me(func: uid(1, 3 , 5, 7)) {
			name @groupby(abc) @groupby(bcd)
		}
	}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
}

func TestFilterError3(t *testing.T) {
	query := `
	{
		me(func: uid(1, 3 , 5, 7)) {
			expand(_all_) @filter(eq(name, "abc"))
		}
	}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
}

func TestFilterUid(t *testing.T) {
	query := `
	{
		me(func: uid(1, 3 , 5, 7)) @filter(uid(3, 7)) {
			name
		}
	}
	`
	gql, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, []uint64{1, 3, 5, 7}, gql.Query[0].UID)
	require.Equal(t, []uint64{3, 7}, gql.Query[0].Filter.Func.UID)
}

func TestIdErr(t *testing.T) {
	query := `
	{
		me(id: [1, 3 , 5, 7]) @filter(uid(3, 7)) {
			name
		}
	}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got invalid keyword: id")
}

func TestFilterVarErr(t *testing.T) {
	query := `
	{
		x as m(func: allofterms(name, "Pawan Rawal"))
	}
	{
		me(func: uid(1, 3 , 5, 7)) @filter(var(x)) {
			name
		}
	}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unexpected var()")
}

func TestEqUidFunctionErr(t *testing.T) {
	query := `
		{
			me(func: eq(path_id, uid(x))) {
				name
			}
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only val/count allowed as function within another. Got: uid")
}

func TestAggRoot1(t *testing.T) {
	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			me() {
				sum(val(a))
				avg(val(a))
			}
		}
	`
	gql, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, "me", gql.Query[1].Alias)
	require.Equal(t, true, gql.Query[1].IsEmpty)
}

func TestAggRootError(t *testing.T) {
	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			me() {
				sum(val(a))
				avg(val(a))
				friend @filter(anyofterms(name, "Hey")) {
					name
				}
			}
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only aggregation/math functions allowed inside empty blocks. Got: friend")
}

func TestAggRootError2(t *testing.T) {
	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			me() {
				avg(val(a))
				name
			}
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only aggregation/math functions allowed inside empty blocks. Got: name")
}

func TestAggRootError3(t *testing.T) {
	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			me() {
				avg
			}
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only aggregation/math functions allowed inside empty blocks. Got: avg")
}

func TestEmptyFunction(t *testing.T) {
	query := `
		{
		  director(func:allofterms()) {
		    name@en
		    director.film (orderdesc: initial_release_date) {
		      name@en
		      initial_release_date
		    }
		  }
		}`

	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got empty attr for function: [allofterms]")
}

func TestOrder1(t *testing.T) {
	query := `
		{
			me(func: uid(1), orderdesc: name, orderasc: age) {
				name
			}
		}
	`
	gq, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, 2, len(gq.Query[0].Order))
	require.Equal(t, "name", gq.Query[0].Order[0].Attr)
	require.Equal(t, true, gq.Query[0].Order[0].Desc)
	require.Equal(t, "age", gq.Query[0].Order[1].Attr)
	require.Equal(t, false, gq.Query[0].Order[1].Desc)
}

func TestOrder2(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				friend(orderasc: alias, orderdesc: name) @filter(lt(alias, "Pat")) {
					alias
				}
			}
		}
	`
	gq, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	curp := gq.Query[0].Children[0]
	require.Equal(t, 2, len(curp.Order))
	require.Equal(t, "alias", curp.Order[0].Attr)
	require.Equal(t, false, curp.Order[0].Desc)
	require.Equal(t, "name", curp.Order[1].Attr)
	require.Equal(t, true, curp.Order[1].Desc)
}

func TestMultipleOrderError(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				friend(orderasc: alias, orderdesc: alias) {
					alias
				}
			}
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Sorting by an attribute: [alias] can only be done once")
}

func TestMultipleOrderError2(t *testing.T) {
	query := `
		{
			me(func: uid(0x01),orderasc: alias, orderdesc: alias) {
				friend {
					alias
				}
			}
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Sorting by an attribute: [alias] can only be done once")
}

func TestEqArgWithDollar(t *testing.T) {
	// This is a fix for #1444.
	query := `
	{
		ab(func: eq(name@en, "$pringfield (or, How)")) {
			uid
		}
	}
	`
	gql, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, gql.Query[0].Func.Args[0].Value, `$pringfield (or, How)`)
}

func TestLangWithDash(t *testing.T) {
	query := `{
		q(func: uid(1)) {
			text@en-us
		}
	}`

	gql, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.Equal(t, []string{"en-us"}, gql.Query[0].Children[0].Langs)
}

func TestOrderByVarAndPred(t *testing.T) {
	query := `{
		q(func: uid(1), orderasc: name, orderdesc: val(n)) {
		}

		var(func: uid(0x0a)) {
			friends {
				n AS name
			}
		}

	}`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Multiple sorting only allowed by predicates.")

	query = `{
		q(func: uid(1)) {
		}

		var(func: uid(0x0a)) {
			friends (orderasc: name, orderdesc: val(n)) {
				n AS name
			}
		}

	}`
	_, err = Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Multiple sorting only allowed by predicates.")

	query = `{
		q(func: uid(1)) {
		}

		var(func: uid(0x0a)) {
			friends (orderasc: name, orderdesc: genre) {
				name
			}
		}

	}`
	_, err = Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
}

func TestInvalidValUsage(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				val(uid) {
					nope
				}
			}
		}
	`
	_, err := Parse(Request{Str: query, Http: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Query syntax invalid.")
}

func TestOrderWithLang(t *testing.T) {
	query := `
	{
		me(func: uid(0x1), orderasc: name@en:fr:., orderdesc: lastname@ci, orderasc: salary) {
			name
		}
	}
`
	res, err := Parse(Request{Str: query, Http: true})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	orders := res.Query[0].Order
	require.Equal(t, "name", orders[0].Attr)
	require.Equal(t, []string{"en", "fr", "."}, orders[0].Langs)
	require.Equal(t, "lastname", orders[1].Attr)
	require.Equal(t, []string{"ci"}, orders[1].Langs)
	require.Equal(t, "salary", orders[2].Attr)
	require.Equal(t, 0, len(orders[2].Langs))
}

func TestParseLangTagAfterStringInRoot(t *testing.T) {
	// This is a fix for #1499.
	query := `
		{
			q(func: anyofterms(name, "Hello"@en)) {
				uid
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid usage of '@' in function argument")
}

func TestParseLangTagAfterStringInFilter(t *testing.T) {
	// This is a fix for #1499.
	query := `
		{
			q(func: uid(0x01)) @filter(eq(name, "Hello"@en)) {
				uid
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid usage of '@' in function argument")
}

func TestParseUidAsArgument(t *testing.T) {
	// This is a fix for #1655 and #1656
	query := `
		{
			q(func: gt(uid, 0)) {
				uid
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Argument cannot be \"uid\"")
}

func TestParseMutation(t *testing.T) {
	m := `
		{
			set {
				<name> <is> <something> .
				<hometown> <is> <san/francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		}
	`
	mu, err := ParseMutation(m)
	require.NoError(t, err)
	require.NotNil(t, mu)
	sets, err := rdf.ConvertToNQuads(string(mu.SetNquads))
	require.NoError(t, err)
	require.EqualValues(t, &protos.NQuad{
		Subject: "name", Predicate: "is", ObjectId: "something"},
		sets[0])
	require.EqualValues(t, &protos.NQuad{
		Subject: "hometown", Predicate: "is", ObjectId: "san/francisco"},
		sets[1])
	dels, err := rdf.ConvertToNQuads(string(mu.DelNquads))
	require.NoError(t, err)
	require.EqualValues(t, &protos.NQuad{
		Subject: "name", Predicate: "is", ObjectId: "something-else"},
		dels[0])

}
