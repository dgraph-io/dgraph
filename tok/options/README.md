In Dgraph, we have the abiltity to specify an @index directive for the type vfloat. Here, we want to
expand that capability to allow us to specify something like:

myAttribute @index("hnsw-euclidean","maxNeighbors":"5","maxLevels":"5","exponent":"6")

Roughly, this should be read as: I want to specify an HNSW-type vector index using euclidean
distance as a metric with maxNeighbors, maxLevels, and exponent being declared as written, and with
all other parameters taking on default values. (In the case of specifying an exponent, this should
affect the defaults for the other options, but that is something that can be decided by the factory
for the hnsw index)!

But this leads to some natural questions: How do I know what option types are allowed? And how do I
interpret their types? For example, how do I know that I should interpret "5" as an integer rather
than a string?

The solution will be to allow each factory to specify a set of "allowed option types". Hence, we get
the AllowedOptions class, which specifies a set of named options and corresponding parsers for the
given name.

Now, if we want to populate an Options instance based on the content of myAttribute
@index(hnsw(metric:"euclidean",exponent:"6")),

we collect the key-value pairs: "metric":"euclidean" "exponent":"6"

And we can now invoke: allowedOpts := hnswFactory.AllowedOpts() myAttributeIndexOpts := NewOptions()

val, err := allowedOpts.GetParsedOption("exponent", "6") if err != nil { return
ErrBadOptionNoBiscuit } myAttributeIndexOpts.SetOpt("exponent", val)

The final resolution of the "puzzle" in fact is to invoke allowedOpts.PopulateOptions(pairs,
myAttributeIndexOpts) based on pairs being built from the collection of key-value pairs.
