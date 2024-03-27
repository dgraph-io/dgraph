// TODO: Consider moving this to a shared location, if it ends up getting used
//       by more than one package. Currently, the target is primarily the
//       vector-indexer, but it might also be applicable to the planned
//       vector-compression algorithms.

For a plugin mechanism, it is critical that all of the starting points for the
plugin have exactly the same function signature so that we can reasonably find
it and invoke it. How then, do we allow for supporting different construction
parameters given that different plugins might have different needs? The options
package is intended to be an answer to that question.

It's best to understand this package by looking at the first target use-case,
which is to specify a Vector Index plugin. Our intent is to start with the
factory: for a given kind of Vector Index, you need to have a factory to Create
and Remove VectorIndexes or to find an index that already exists.

If we only cared about the HNSW-type of VectorIndex, this would not be an issue,
but if we want the ability to support different kinds of VectorIndex
implementations, or to let customers specify their own, we need to make sure
that we have a consistent mechanism for creating a factory that applies to all
of them -- despite the fact that internally, they might all care about very
different kinds of parameters.

Our choice here is to let the factory itself be built using no arguments, but
for the VectorIndex lifetime operations (specifically, the operations involved
in creating a new VectorIndex) to specify a set of "generic" options.

Hence, we now see the point of the "Options" type: It is basically a map
of option name to a specific value.

In Dgraph, we have the abiltity to specify an @index directive for the type
vfloat. Here, we want to expand that capability to allow us to specify something
like:

myAttribute @index("hnsw-euclidian","maxNeighbors":"5","maxLevels":"5","exponent":"6")

Roughly, this should be read as:
I want to specify an HNSW-type vector index using euclidian distance as a metric
with maxNeighbors, maxLevels, and exponent being declared as written, and with
all other parameters taking on default values.  (In the case of specifying an
exponent, this should affect the defaults for the other options, but that is
something that can be decided by the factory for the hnsw index)!

But this leads to some natural questions: How do I know what option types are
allowed? And how do I interpret their types? For example, how do I know that I
should interpret "5" as an integer rather than a string?

The solution will be to allow each factory to specify a set of "allowed option
types". Hence, we get the AllowedOptions class, which specifies a set of
named options and corresponding parsers for the given name.

Now, if we want to populate an Options instance based on the content of
myAttribute @index(hnsw(metric:"euclidian",exponent:"6")),

we collect the key-value pairs:
   "metric":"euclidian"
   "exponent":"6"

And we can now invoke:
allowedOpts := hnswFactory.AllowedOpts()
myAttributeIndexOpts := NewOptions()

val, err := allowedOpts.GetParsedOption("exponent", "6")
if err != nil {
   return ErrBadOptionNoBiscuit
}
myAttributeIndexOpts.SetOpt("exponent", val)

The final resolution of the "puzzle" in fact is to invoke
allowedOpts.PopulateOptions(pairs, myAttributeIndexOpts)
based on pairs being built from the collection of key-value pairs.
