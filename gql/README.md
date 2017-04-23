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

Current Parser
Benchmark_format1-4                       100000             12768 ns/op            6306 B/op         55 allocs/op
Benchmark_format2-4                       100000             20439 ns/op            7314 B/op         60 allocs/op
Benchmark_format3-4                       100000             15317 ns/op            6242 B/op         50 allocs/op
Benchmark_directors-4                     100000             21481 ns/op            9320 B/op         86 allocs/op
Benchmark_Movies-4                        100000             18335 ns/op            8536 B/op         78 allocs/op
Benchmark_Filters-4                       100000             14746 ns/op            6904 B/op         74 allocs/op
Benchmark_Geq-4                           100000             20107 ns/op            6480 B/op         62 allocs/op
Benchmark_Date-4                          100000             13957 ns/op            6696 B/op         58 allocs/op
Benchmark_Mutation-4                      200000              6180 ns/op            4184 B/op         24 allocs/op
Benchmark_Mutation1000-4                    2000            584556 ns/op          134873 B/op         24 allocs/op
Benchmark_format1_parallel-4              200000             36625 ns/op            6308 B/op         55 allocs/op
Benchmark_format2_parallel-4              100000             13553 ns/op            7317 B/op         60 allocs/op
Benchmark_format3_parallel-4              100000             12978 ns/op            6244 B/op         50 allocs/op
Benchmark_directors_parallel-4            100000             34461 ns/op            9320 B/op         86 allocs/op
Benchmark_Movies_parallel-4               100000             62705 ns/op            8536 B/op         78 allocs/op
Benchmark_Filters_parallel-4              100000             10700 ns/op            6904 B/op         74 allocs/op
Benchmark_Geq_parallel-4                  200000             36977 ns/op            6480 B/op         62 allocs/op
Benchmark_Date_parallel-4                 200000             10280 ns/op            6696 B/op         58 allocs/op
Benchmark_Mutation_parallel-4             300000              5944 ns/op            4184 B/op         24 allocs/op
Benchmark_Mutation1000_parallel-4          10000            204572 ns/op          134874 B/op         24 allocs/op

-------
Benchmark_format1-4                       100000             11308 ns/op            5690 B/op         44 allocs/op
Benchmark_format2-4                       100000             16375 ns/op            6570 B/op         54 allocs/op
Benchmark_format3-4                       100000             15312 ns/op            6138 B/op         46 allocs/op
Benchmark_directors-4                     100000             19523 ns/op            8336 B/op         74 allocs/op
Benchmark_Movies-4                        100000             22019 ns/op            7648 B/op         66 allocs/op
Benchmark_Filters-4                       100000             13793 ns/op            6160 B/op         62 allocs/op
Benchmark_Geq-4                           100000             12268 ns/op            5768 B/op         50 allocs/op
Benchmark_Date-4                          100000             12037 ns/op            5952 B/op         46 allocs/op
Benchmark_Mutation-4                      300000              4946 ns/op            3504 B/op         13 allocs/op
Benchmark_Mutation1000-4                    3000            554701 ns/op           68848 B/op         13 allocs/op
Benchmark_format1_parallel-4              200000              8181 ns/op            5692 B/op         44 allocs/op
Benchmark_format2_parallel-4              200000             11575 ns/op            6572 B/op         54 allocs/op
Benchmark_format3_parallel-4              200000             10965 ns/op            6140 B/op         46 allocs/op
Benchmark_directors_parallel-4            100000             14911 ns/op            8336 B/op         74 allocs/op
Benchmark_Movies_parallel-4               100000             11774 ns/op            7648 B/op         66 allocs/op
Benchmark_Filters_parallel-4              200000              9371 ns/op            6160 B/op         62 allocs/op
Benchmark_Geq_parallel-4                  200000              8320 ns/op            5768 B/op         50 allocs/op
Benchmark_Date_parallel-4                 200000              9522 ns/op            5952 B/op         46 allocs/op
Benchmark_Mutation_parallel-4             300000              5145 ns/op            3504 B/op         13 allocs/op
Benchmark_Mutation1000_parallel-4          10000            169688 ns/op           68849 B/op         13 allocs/op


Benchmark_Format1_sec-4           300000             12311 ns/op            6306 B/op         55 allocs/op
Benchmark_Format2_sec-4           200000             19583 ns/op            7314 B/op         60 allocs/op
Benchmark_Format3_sec-4           300000             16202 ns/op            6242 B/op         50 allocs/op
Benchmark_directors_sec-4         200000             23105 ns/op            9320 B/op         86 allocs/op
Benchmark_Movies_sec-4            200000             18761 ns/op            8536 B/op         78 allocs/op
Benchmark_Filters_sec-4           300000             18149 ns/op            6904 B/op         74 allocs/op
Benchmark_Geq_sec-4               300000             13504 ns/op            6480 B/op         62 allocs/op
Benchmark_Date_sec-4              300000             14747 ns/op            6696 B/op         58 allocs/op
Benchmark_Mutation_sec-4          500000              6960 ns/op            4184 B/op         24 allocs/op
Benchmark_Mutation1000_sec-4       10000            596270 ns/op          134873 B/op         24 allocs/op

Benchmark_Format1_sec-4           300000             11905 ns/op            5690 B/op         44 allocs/op
Benchmark_Format2_sec-4           300000             16503 ns/op            6570 B/op         54 allocs/op
Benchmark_Format3_sec-4           300000             14889 ns/op            6138 B/op         46 allocs/op
Benchmark_directors_sec-4         200000             20714 ns/op            8336 B/op         74 allocs/op
Benchmark_Movies_sec-4            300000             17112 ns/op            7648 B/op         66 allocs/op
Benchmark_Filters_sec-4           300000             17536 ns/op            6160 B/op         62 allocs/op
Benchmark_Geq_sec-4               300000             12397 ns/op            5768 B/op         50 allocs/op
Benchmark_Date_sec-4              300000             14596 ns/op            5952 B/op         46 allocs/op
Benchmark_Mutation_sec-4         1000000              5930 ns/op            3504 B/op         13 allocs/op
Benchmark_Mutation1000_sec-4       10000            557493 ns/op           68848 B/op         13 allocs/op

Benchmark_format1_parallel-4              200000             36625 ns/op            6308 B/op         55 allocs/op
Benchmark_format2_parallel-4              100000             13553 ns/op            7317 B/op         60 allocs/op
Benchmark_format3_parallel-4              100000             12978 ns/op            6244 B/op         50 allocs/op
Benchmark_directors_parallel-4            100000             34461 ns/op            9320 B/op         86 allocs/op
Benchmark_Movies_parallel-4               100000             62705 ns/op            8536 B/op         78 allocs/op
Benchmark_Filters_parallel-4              100000             10700 ns/op            6904 B/op         74 allocs/op
Benchmark_Geq_parallel-4                  200000             36977 ns/op            6480 B/op         62 allocs/op
Benchmark_Date_parallel-4                 200000             10280 ns/op            6696 B/op         58 allocs/op
Benchmark_Mutation_parallel-4             300000              5944 ns/op            4184 B/op         24 allocs/op
Benchmark_Mutation1000_parallel-4          10000            204572 ns/op          134874 B/op         24 allocs/op

Benchmark_format1_parallel-4              200000              8181 ns/op            5692 B/op         44 allocs/op
Benchmark_format2_parallel-4              200000             11575 ns/op            6572 B/op         54 allocs/op
Benchmark_format3_parallel-4              200000             10965 ns/op            6140 B/op         46 allocs/op
Benchmark_directors_parallel-4            100000             14911 ns/op            8336 B/op         74 allocs/op
Benchmark_Movies_parallel-4               100000             11774 ns/op            7648 B/op         66 allocs/op
Benchmark_Filters_parallel-4              200000              9371 ns/op            6160 B/op         62 allocs/op
Benchmark_Geq_parallel-4                  200000              8320 ns/op            5768 B/op         50 allocs/op
Benchmark_Date_parallel-4                 200000              9522 ns/op            5952 B/op         46 allocs/op
Benchmark_Mutation_parallel-4             300000              5145 ns/op            3504 B/op         13 allocs/op
Benchmark_Mutation1000_parallel-4          10000            169688 ns/op           68849 B/op         13 allocs/op
