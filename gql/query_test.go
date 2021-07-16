/*
 * Copyright 2015-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gql

import (
	"testing"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/stretchr/testify/require"
)

// This file contains the tests of format TestParseQuery* and Mutations.
func TestParseQueryNamedQuery(t *testing.T) {
	query := `
query works() {
  q(func: has(name)) {
    name
  }
}
`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestParseQueryNameQueryWithoutBrackers(t *testing.T) {
	query := `
query works {
  q(func: has(name)) {
    name
  }
}
`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestDuplicateQueryAliasesError(t *testing.T) {
	query := `
{
  find_michael(func: eq(name@., "Michael")) {
    uid
    name@.
    age
  }
    find_michael(func: eq(name@., "Amit")) {
      uid
      name@.
    }
}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)

	queryInOpType := `
{
  find_michael(func: eq(name@., "Michael")) {
    uid
    name@.
    age
  }
}
query {find_michael(func: eq(name@., "Amit")) {
      uid
      name@.
    }
}
`
	_, err = Parse(Request{Str: queryInOpType})
	require.Error(t, err)

	queryWithDuplicateShortestPaths := `
{
 path as shortest(from: 0x1, to: 0x4) {
  friend
 }
 path2 as shortest(from: 0x2, to: 0x3) {
    friend
 }
 pathQuery1(func: uid(path)) {
   name
 }
 pathQuery2(func: uid(path2)) {
   name
 }

}`
	_, err = Parse(Request{Str: queryWithDuplicateShortestPaths})
	require.NoError(t, err)
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
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestParseQueryExpandForward(t *testing.T) {
	query := `
	{
		var(func: uid( 0x0a)) {
			friends {
				expand(_forward_)
			}
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Argument _forward_ has been deprecated")
}

func TestParseQueryExpandReverse(t *testing.T) {
	query := `
	{
		var(func: uid( 0x0a)) {
			friends {
				expand(_reverse_)
			}
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Argument _reverse_ has been deprecated")
}

func TestParseQueryExpandType(t *testing.T) {
	query := `
	{
		var(func: uid( 0x0a)) {
			friends {
				expand(Person)
			}
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestParseQueryExpandMultipleTypes(t *testing.T) {
	query := `
	{
		var(func: uid( 0x0a)) {
			friends {
				expand(Person, Relative)
			}
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestParseQueryAliasListPred(t *testing.T) {
	query := `
	{
		me(func: uid(0x0a)) {
			pred: some_pred
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, "pred", res.Query[0].Children[0].Alias)
	require.Equal(t, "some_pred", res.Query[0].Children[0].Attr)
}

func TestParseQueryCountListPred(t *testing.T) {
	query := `
	{
		me(func: uid(0x0a)) {
			count(some_pred)
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, true, res.Query[0].Children[0].IsCount)
	require.Equal(t, "some_pred", res.Query[0].Children[0].Attr)
}

func TestParseQueryListPred2(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			f as friends
		}

		var(func: uid(f)) {
			l as some_pred
		}

		var(func: uid( 0x0a)) {
			friends {
				expand(val(l))
			}
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestParseQueryListPred_MultiVarError(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			f as friends
		}

		var(func: uid(f)) {
			l as some_pred
			friend {
				g as some_pred
			}
		}

		var(func: uid( 0x0a)) {
			friends {
				expand(val(l, g))
			}
		}
	}
`
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	res, err := Parse(Request{Str: query})
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
				d as math(exp(a + b + 1.0) - ln(c))
				q as math(c*-1.0+-b+(-b*c))
			}
		}
	}
`
	res, err := Parse(Request{Str: query})
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
				d as math(exp(a + b + 1.0) - max(c,ln(c)) + sqrt(a%b))
			}
		}
	}
`
	res, err := Parse(Request{Str: query})
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
	res, err := Parse(Request{Str: query})
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
				d as math(cond(a <= 10.0, exp(a + b + 1.0), ln(c)) + 10*a)
				e as math(cond(a!=10.0, exp(a + b + 1.0), ln(d)))
				f as math(cond(a==10.0, exp(a + b + 1.0), ln(e)))
			}
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.EqualValues(t, "(+ (cond (<= a 1E+01) (exp (+ (+ a b) 1E+00)) (ln c)) (* 10 a))",
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
				d as math(a + b * c / a + exp(a + b + 1.0) - ln(c))
			}
		}
	}
`
	res, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "a", res.Query[0].Children[0].Children[0].Var)
	require.True(t, res.Query[0].Children[1].IsInternal)
	require.Equal(t, "a", res.Query[0].Children[1].NeedsVar[0].Name)
	require.Equal(t, ValueVar, res.Query[0].Children[1].NeedsVar[0].Typ)
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0].Name)
	require.Equal(t, UidVar, res.Query[0].NeedsVar[0].Typ)
	require.Equal(t, "c", res.Query[0].NeedsVar[1].Name)
	require.Equal(t, ValueVar, res.Query[0].NeedsVar[1].Typ)
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0].Name)
	require.Equal(t, UidVar, res.Query[0].NeedsVar[0].Typ)
	require.Equal(t, "n", res.Query[0].NeedsVar[1].Name)
	require.Equal(t, ValueVar, res.Query[0].NeedsVar[1].Typ)
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0].Name)
	require.Equal(t, UidVar, res.Query[0].NeedsVar[0].Typ)
	require.Equal(t, "n", res.Query[0].NeedsVar[1].Name)
	require.Equal(t, ValueVar, res.Query[0].NeedsVar[1].Typ)
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0].Name)
	require.Equal(t, UidVar, res.Query[0].NeedsVar[0].Typ)
	require.Equal(t, "n", res.Query[0].NeedsVar[1].Name)
	require.Equal(t, ValueVar, res.Query[0].NeedsVar[1].Typ)
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 4, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0].Name)
	require.Equal(t, "J", res.Query[0].NeedsVar[1].Name)
	require.Equal(t, "K", res.Query[0].NeedsVar[2].Name)
	require.Equal(t, UidVar, res.Query[0].NeedsVar[0].Typ)
	require.Equal(t, UidVar, res.Query[0].NeedsVar[1].Typ)
	require.Equal(t, UidVar, res.Query[0].NeedsVar[2].Typ)
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 6, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0].Name)
	require.Equal(t, "J", res.Query[1].NeedsVar[0].Name)
	require.Equal(t, "K", res.Query[2].NeedsVar[0].Name)
	require.Equal(t, UidVar, res.Query[0].NeedsVar[0].Typ)
	require.Equal(t, UidVar, res.Query[1].NeedsVar[0].Typ)
	require.Equal(t, UidVar, res.Query[2].NeedsVar[0].Typ)
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	_, err := Parse(Request{Str: query})
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "K", res.Query[0].Var)
	require.Equal(t, "L", res.Query[0].Children[0].Var)
	require.Equal(t, "L", res.Query[1].Filter.Func.NeedsVar[0].Name)
	require.Equal(t, UidVar, res.Query[1].Filter.Func.NeedsVar[0].Typ)
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "K", res.Query[0].Var)
	require.Equal(t, "fr", res.Query[0].Children[0].Var)
	require.Equal(t, "fr", res.Query[1].NeedsVar[0].Name)
	require.Equal(t, UidVar, res.Query[1].NeedsVar[0].Typ)
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
	_, err := Parse(Request{Str: query})
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "fr", res.Query[0].Children[0].Var)
	require.Equal(t, "fr", res.Query[1].NeedsVar[0].Name)
	require.Equal(t, UidVar, res.Query[1].NeedsVar[0].Typ)
	require.Equal(t, ValueVar, res.Query[1].Filter.Func.NeedsVar[0].Typ)
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "L", res.Query[0].Children[0].Var)
	require.Equal(t, "L", res.Query[1].NeedsVar[0].Name)
	require.Equal(t, UidVar, res.Query[1].NeedsVar[0].Typ)
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 3, len(res.Query))
	require.Equal(t, "L", res.Query[0].Children[0].Var)
	require.Equal(t, "B", res.Query[0].Children[0].Children[0].Var)
	require.Equal(t, "L", res.Query[1].NeedsVar[0].Name)
	require.Equal(t, "B", res.Query[2].NeedsVar[0].Name)
	require.Equal(t, UidVar, res.Query[1].NeedsVar[0].Typ)
	require.Equal(t, UidVar, res.Query[2].NeedsVar[0].Typ)
	require.Equal(t, []string{"L", "B"}, res.QueryVars[0].Defines)
	require.Equal(t, []string{"L"}, res.QueryVars[1].Needs)
	require.Equal(t, []string{"B"}, res.QueryVars[2].Needs)
}

func TestParseQueryWithAttrLang(t *testing.T) {
	query := `
	{
		me(func: uid(0x1)) {
			name
			friend(first:5, orderasc: name@en) {
				name@en
			}
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, "name", res.Query[0].Children[1].Order[0].Attr)
	require.Equal(t, []string{"en"}, res.Query[0].Children[1].Order[0].Langs)
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, "name", res.Query[0].Order[0].Attr)
	require.Equal(t, []string{"en"}, res.Query[0].Order[0].Langs)
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
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Contains(t, err.Error(), `Invalid block: [mutation]`)
}

func TestParseMutationError2(t *testing.T) {
	query := `
			set {
				<name> <is> <something> .
				<hometown> <is> <san/francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
	`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Contains(t, err.Error(), `Invalid block: [set]`)
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
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
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
	req, err := ParseMutation(m)
	require.NoError(t, err)
	mu := req.Mutations[0]
	require.NotNil(t, mu)
	sets, err := parseNquads(mu.SetNquads)
	require.NoError(t, err)
	require.EqualValues(t, &api.NQuad{
		Subject: "name", Predicate: "is", ObjectId: "something"},
		sets[0])
	require.EqualValues(t, &api.NQuad{
		Subject: "hometown", Predicate: "is", ObjectId: "san/francisco"},
		sets[1])
	dels, err := parseNquads(mu.DelNquads)
	require.NoError(t, err)
	require.EqualValues(t, &api.NQuad{
		Subject: "name", Predicate: "is", ObjectId: "something-else"},
		dels[0])
}

func TestParseMutationTooManyBlocks(t *testing.T) {
	tests := []struct {
		m      string
		errStr string
	}{
		{m: `
         {
           set { _:a1 <reg> "a1 content" . }
         }{
		   set { _:b2 <reg> "b2 content" . }
		 }`,
			errStr: "Unrecognized character in lexText",
		},
		{m: `{set { _:a1 <reg> "a1 content" . }} something`,
			errStr: "Invalid operation type: something",
		},
		{m: `
          # comments are ok
		  {
		    set { _:a1 <reg> "a1 content" . } # comments are ok
          } # comments are ok`,
		},
	}
	for _, tc := range tests {
		mu, err := ParseMutation(tc.m)
		if tc.errStr != "" {
			require.Contains(t, err.Error(), tc.errStr)
			require.Nil(t, mu)
		} else {
			require.NoError(t, err)
		}
	}
}
