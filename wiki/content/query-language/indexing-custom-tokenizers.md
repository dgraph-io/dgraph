+++
date = "2017-03-20T22:25:17+11:00"
title = "Indexing with Custom Tokenizers"
weight = 27
[menu.main]
    parent = "query-language"
+++

Dgraph comes with a large toolkit of builtin indexes, but sometimes for niche
use cases they're not always enough.

Dgraph allows you to implement custom tokenizers via a plugin system in order
to fill the gaps.

## Caveats

The plugin system uses Go's [`pkg/plugin`](https://golang.org/pkg/plugin/).
This brings some restrictions to how plugins can be used.

- Plugins must be written in Go.

- As of Go 1.9, `pkg/plugin` only works on Linux. Therefore, plugins will only
  work on Dgraph instances deployed in a Linux environment.

- The version of Go used to compile the plugin should be the same as the version
  of Go used to compile Dgraph itself. Dgraph always uses the latest version of
Go (and so should you!).

## Implementing a plugin

{{% notice "note" %}}
You should consider Go's [plugin](https://golang.org/pkg/plugin/) documentation
to be supplementary to the documentation provided here.
{{% /notice %}}

Plugins are implemented as their own main package. They must export a
particular symbol that allows Dgraph to hook into the custom logic the plugin
provides.

The plugin must export a symbol named `Tokenizer`. The type of the symbol must
be `func() interface{}`. When the function is called the result returned should
be a value that implements the following interface:

```
type PluginTokenizer interface {
    // Name is the name of the tokenizer. It should be unique among all
    // builtin tokenizers and other custom tokenizers. It identifies the
    // tokenizer when an index is set in the schema and when search/filter
    // is used in queries.
    Name() string

    // Identifier is a byte that uniquely identifiers the tokenizer.
    // Bytes in the range 0x80 to 0xff (inclusive) are reserved for
    // custom tokenizers.
    Identifier() byte

    // Type is a string representing the type of data that is to be
    // tokenized. This must match the schema type of the predicate
    // being indexed. Allowable values are shown in the table below.
    Type() string

    // Tokens should implement the tokenization logic. The input is
    // the value to be tokenized, and will always have a concrete type
    // corresponding to Type(). The return value should be a list of
    // the tokens generated.
    Tokens(interface{}) ([]string, error)
}
```

The return value of `Type()` corresponds to the concrete input type of
`Tokens(interface{})` in the following way:

 `Type()` return value | `Tokens(interface{})` input type
-----------------------|----------------------------------
 `"int"`               | `int64`
 `"float"`             | `float64`
 `"string"`            | `string`
 `"bool"`              | `bool`
 `"datetime"`          | `time.Time`

## Building the plugin

The plugin has to be built using the `plugin` build mode so that an `.so` file
is produced instead of a regular executable. For example:

```sh
go build -buildmode=plugin -o myplugin.so ~/go/src/myplugin/main.go
```

## Running Dgraph with plugins

When starting Dgraph, use the `--custom_tokenizers` flag to tell Dgraph which
tokenizers to load. It accepts a comma separated list of plugins. E.g.

```sh
dgraph ...other-args... --custom_tokenizers=plugin1.so,plugin2.so
```

{{% notice "note" %}}
Plugin validation is performed on startup. If a problem is detected, Dgraph
will refuse to initialise.
{{% /notice %}}

## Adding the index to the schema

To use a tokenization plugin, an index has to be created in the schema.

The syntax is the same as adding any built-in index. To add an custom index
using a tokenizer plugin named `foo` to a `string` predicate named
`my_predicate`, use the following in the schema:

```sh
my_predicate: string @index(foo) .
```

## Using the index in queries

There are two functions that can use custom indexes:

 Mode | Behaviour
--------|-------
 `anyof` | Returns nodes that match on *any* of the tokens generated
 `allof` | Returns nodes that match on *all* of the tokens generated

The functions can be used either at the query root or in filters.

There behaviour here an analogous to `anyofterms`/`allofterms` and
`anyoftext`/`alloftext`.

## Examples

The following examples should make the process of writing a tokenization plugin
more concrete.

### Unicode Characters

This example shows the type of tokenization that is similar to term
tokenization of full-text search. Instead of being broken down into terms or
stem words, the text is instead broken down into its constituent unicode
codepoints (in Go terminology these are called *runes*).

{{% notice "note" %}}
This tokenizer would create a very large index that would be expensive to
manage and store. That's one of the reasons that text indexing usually occurs
at a higher level; stem words for full-text search or terms for term search.
{{% /notice %}}

The implementation of the plugin looks like this:

```go
package main

import "encoding/binary"

func Tokenizer() interface{} { return RuneTokenizer{} }

type RuneTokenizer struct{}

func (RuneTokenizer) Name() string     { return "rune" }
func (RuneTokenizer) Type() string     { return "string" }
func (RuneTokenizer) Identifier() byte { return 0xfd }

func (t RuneTokenizer) Tokens(value interface{}) ([]string, error) {
	var toks []string
	for _, r := range value.(string) {
		var buf [binary.MaxVarintLen32]byte
		n := binary.PutVarint(buf[:], int64(r))
		tok := string(buf[:n])
		toks = append(toks, tok)
	}
	return toks, nil
}
```

**Hints and tips:**

- Inside `Tokens`, you can assume that `value` will have concrete type
  corresponding to that specified by `Type()`. It's safe to do a type
assertion.

- Even though the return value is `[]string`, you can always store non-unicode
  data inside the string. See [this blogpost](https://blog.golang.org/strings)
for some interesting background how string are implemented in Go and why they
can be used to store non-textual data. By storing arbitrary data in the string,
you can make the index more compact. In this case, varints are stored in the
return values.

Setting up the indexing and adding data:
```
name: string @index(rune) .
```


```
{
  set{
    _:ad <name> "Adam" .
    _:ad <dgraph.type> "Person" .
    _:aa <name> "Aaron" .
    _:aa <dgraph.type> "Person" .
    _:am <name> "Amy" .
    _:am <dgraph.type> "Person" .
    _:ro <name> "Ronald" .
    _:ro <dgraph.type> "Person" .
  }
}
```
Now queries can be performed.

The only person that has all of the runes `A` and `n` in their `name` is Aaron:
```
{
  q(func: allof(name, rune, "An")) {
    name
  }
}
=>
{
  "data": {
    "q": [
      { "name": "Aaron" }
    ]
  }
}
```
But there are multiple people who have both of the runes `A` and `m`:
```
{
  q(func: allof(name, rune, "Am")) {
    name
  }
}
=>
{
  "data": {
    "q": [
      { "name": "Amy" },
      { "name": "Adam" }
    ]
  }
}
```
Case is taken into account, so if you search for all names containing `"ron"`,
you would find `"Aaron"`, but not `"Ronald"`. But if you were to search for
`"no"`, you would match both `"Aaron"` and `"Ronald"`. The order of the runes in
the strings doesn't matter.

It's possible to search for people that have *any* of the supplied runes in
their names (rather than *all* of the supplied runes). To do this, use `anyof`
instead of `allof`:
```
{
  q(func: anyof(name, rune, "mr")) {
    name
  }
}
=>
{
  "data": {
    "q": [
      { "name": "Adam" },
      { "name": "Aaron" },
      { "name": "Amy" }
    ]
  }
}
```
`"Ronald"` doesn't contain `m` or `r`, so isn't found by the search.

{{% notice "note" %}}
Understanding what's going on under the hood can help you intuitively
understand how `Tokens` method should be implemented.

When Dgraph sees new edges that are to be indexed by your tokenizer, it
will tokenize the value. The resultant tokens are used as keys for posting
lists. The edge subject is then added to the posting list for each token.

When a query root search occurs, the search value is tokenized. The result of
the search is all of the nodes in the union or intersection of the corresponding
posting lists (depending on whether `anyof` or `allof` was used).
{{% /notice %}}

### CIDR Range

Tokenizers don't always have to be about splitting text up into its constituent
parts. This example indexes [IP addresses into their CIDR
ranges](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing). This
allows you to search for all IP addresses that fall into a particular CIDR
range.

The plugin code is more complicated than the rune example. The input is an IP
address stored as a string, e.g. `"100.55.22.11/32"`. The output are the CIDR
ranges that the IP address could possibly fall into. There could be up to 32
different outputs (`"100.55.22.11/32"` does indeed have 32 possible ranges, one
for each mask size).

```go
package main

import "net"

func Tokenizer() interface{} { return CIDRTokenizer{} }

type CIDRTokenizer struct{}

func (CIDRTokenizer) Name() string     { return "cidr" }
func (CIDRTokenizer) Type() string     { return "string" }
func (CIDRTokenizer) Identifier() byte { return 0xff }

func (t CIDRTokenizer) Tokens(value interface{}) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(value.(string))
	if err != nil {
		return nil, err
	}
	ones, bits := ipnet.Mask.Size()
	var toks []string
	for i := ones; i >= 1; i-- {
		m := net.CIDRMask(i, bits)
		tok := net.IPNet{
			IP:   ipnet.IP.Mask(m),
			Mask: m,
		}
		toks = append(toks, tok.String())
	}
	return toks, nil
}
```
An example of using the tokenizer:

Setting up the indexing and adding data:
```
ip: string @index(cidr) .

```

```
{
  set{
    _:a <ip> "100.55.22.11/32" .
    _:b <ip> "100.33.81.19/32" .
    _:c <ip> "100.49.21.25/32" .
    _:d <ip> "101.0.0.5/32" .
    _:e <ip> "100.176.2.1/32" .
  }
}
```
```
{
  q(func: allof(ip, cidr, "100.48.0.0/12")) {
    ip
  }
}
=>
{
  "data": {
    "q": [
      { "ip": "100.55.22.11/32" },
      { "ip": "100.49.21.25/32" }
    ]
  }
}
```
The CIDR ranges of `100.55.22.11/32` and `100.49.21.25/32` are both
`100.48.0.0/12`.  The other IP addresses in the database aren't included in the
search result, since they have different CIDR ranges for 12 bit masks
(`100.32.0.0/12`, `101.0.0.0/12`, `100.154.0.0/12` for `100.33.81.19/32`,
`101.0.0.5/32`, and `100.176.2.1/32` respectively).

Note that we're using `allof` instead of `anyof`. Only `allof` will work
correctly with this index. Remember that the tokenizer generates all possible
CIDR ranges for an IP address. If we were to use `anyof` then the search result
would include all IP addresses under the 1 bit mask (in this case, `0.0.0.0/1`,
which would match all IPs in this dataset).

### Anagram

Tokenizers don't always have to return multiple tokens. If you just want to
index data into groups, have the tokenizer just return an identifying member of
that group.

In this example, we want to find groups of words that are
[anagrams](https://en.wikipedia.org/wiki/Anagram) of each
other.

A token to correspond to a group of anagrams could just be the letters in the
anagram in sorted order, as implemented below:

```go
package main

import "sort"

func Tokenizer() interface{} { return AnagramTokenizer{} }

type AnagramTokenizer struct{}

func (AnagramTokenizer) Name() string     { return "anagram" }
func (AnagramTokenizer) Type() string     { return "string" }
func (AnagramTokenizer) Identifier() byte { return 0xfc }

func (t AnagramTokenizer) Tokens(value interface{}) ([]string, error) {
	b := []byte(value.(string))
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	return []string{string(b)}, nil
}
```
In action:

Setting up the indexing and adding data:
```
word: string @index(anagram) .
```

```
{
  set{
    _:1 <word> "airmen" .
    _:2 <word> "marine" .
    _:3 <word> "beat" .
    _:4 <word> "beta" .
    _:5 <word> "race" .
    _:6 <word> "care" .
  }
}
```
```
{
  q(func: allof(word, anagram, "remain")) {
    word
  }
}
=>
{
  "data": {
    "q": [
      { "word": "airmen" },
      { "word": "marine" }
    ]
  }
}
```

Since a single token is only ever generated, it doesn't matter if `anyof` or
`allof` is used. The result will always be the same.

### Integer prime factors

All of the custom tokenizers shown previously have worked with strings.
However, other data types can be used as well. This example is contrived, but
nonetheless shows some advanced usages of custom tokenizers.

The tokenizer creates a token for each prime factor in the input.

```
package main

import (
    "encoding/binary"
    "fmt"
)

func Tokenizer() interface{} { return FactorTokenizer{} }

type FactorTokenizer struct{}

func (FactorTokenizer) Name() string     { return "factor" }
func (FactorTokenizer) Type() string     { return "int" }
func (FactorTokenizer) Identifier() byte { return 0xfe }

func (FactorTokenizer) Tokens(value interface{}) ([]string, error) {
    x := value.(int64)
    if x <= 1 {
        return nil, fmt.Errorf("Cannot factor int <= 1: %d", x)
    }
    var toks []string
    for p := int64(2); x > 1; p++ {
        if x%p == 0 {
            toks = append(toks, encodeInt(p))
            for x%p == 0 {
                x /= p
            }
        }
    }
    return toks, nil

}

func encodeInt(x int64) string {
    var buf [binary.MaxVarintLen64]byte
    n := binary.PutVarint(buf[:], x)
    return string(buf[:n])
}
```
{{% notice "note" %}}
Notice that the return of `Type()` is `"int"`, corresponding to the concrete
type of the input to `Tokens` (which is `int64`).
{{% /notice %}}

This allows you do things like search for all numbers that share prime
factors with a particular number.

In particular, we search for numbers that contain any of the prime factors of
15, i.e. any numbers that are divisible by either 3 or 5.

Setting up the indexing and adding data:
```
num: int @index(factor) .
```

```
{
  set{
    _:2 <num> "2"^^<xs:int> .
    _:3 <num> "3"^^<xs:int> .
    _:4 <num> "4"^^<xs:int> .
    _:5 <num> "5"^^<xs:int> .
    _:6 <num> "6"^^<xs:int> .
    _:7 <num> "7"^^<xs:int> .
    _:8 <num> "8"^^<xs:int> .
    _:9 <num> "9"^^<xs:int> .
    _:10 <num> "10"^^<xs:int> .
    _:11 <num> "11"^^<xs:int> .
    _:12 <num> "12"^^<xs:int> .
    _:13 <num> "13"^^<xs:int> .
    _:14 <num> "14"^^<xs:int> .
    _:15 <num> "15"^^<xs:int> .
    _:16 <num> "16"^^<xs:int> .
    _:17 <num> "17"^^<xs:int> .
    _:18 <num> "18"^^<xs:int> .
    _:19 <num> "19"^^<xs:int> .
    _:20 <num> "20"^^<xs:int> .
    _:21 <num> "21"^^<xs:int> .
    _:22 <num> "22"^^<xs:int> .
    _:23 <num> "23"^^<xs:int> .
    _:24 <num> "24"^^<xs:int> .
    _:25 <num> "25"^^<xs:int> .
    _:26 <num> "26"^^<xs:int> .
    _:27 <num> "27"^^<xs:int> .
    _:28 <num> "28"^^<xs:int> .
    _:29 <num> "29"^^<xs:int> .
    _:30 <num> "30"^^<xs:int> .
  }
}
```
```
{
  q(func: anyof(num, factor, 15)) {
    num
  }
}
=>
{
  "data": {
    "q": [
      { "num": 3 },
      { "num": 5 },
      { "num": 6 },
      { "num": 9 },
      { "num": 10 },
      { "num": 12 },
      { "num": 15 },
      { "num": 18 }
      { "num": 20 },
      { "num": 21 },
      { "num": 25 },
      { "num": 24 },
      { "num": 27 },
      { "num": 30 },
    ]
  }
}
```
