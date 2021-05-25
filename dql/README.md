Comparing the old and new methods, we find that using slice makes the parsing 20% faster on 
average than using channels. Also, using slices allows the parser to backtrack and peek the 
tokens which couldn't be done using channels as each token can only be consumed once.

```
Name                              unit    Old     New     Improvement
----------------------------------------------------------------------
Benchmark_Filters-4               ns/op   14007   9634    31 %
Benchmark_Geq-4                   ns/op   11701   8602    26 %
Benchmark_Date-4                  ns/op   11687   8630    26 %
Benchmark_directors-4             ns/op   18663   14201   23 %
Benchmark_Filters_parallel-4      ns/op   6486    5015    22 %
Benchmark_Movies-4                ns/op   16097   12807   20 %
Benchmark_directors_parallel-4    ns/op   8766    6966    20 %
Benchmark_Mutation-4              ns/op   5167    4155    19 %
Benchmark_Movies_parallel-4       ns/op   7537    6151    18 %
Benchmark_Date_parallel-4         ns/op   5462    4515    17 %
Benchmark_Geq_parallel-4          ns/op   5390    4485    16 %
Benchmark_Mutation_parallel-4     ns/op   2326    2161    07 %
Benchmark_Mutation1000-4          ns/op   549428  512851  06 %
Benchmark_Mutation1000_parallel-4 ns/op   261785  254911  02 %
```
