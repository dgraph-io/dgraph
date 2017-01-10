Using slice (new):
------------
Benchmark_directors-4            	  100000	     14201 ns/op
Benchmark_Movies-4               	  100000	     12807 ns/op
Benchmark_Filters-4              	  200000	      9634 ns/op
Benchmark_Geq-4                  	  200000	      8602 ns/op
Benchmark_Date-4                 	  200000	      8630 ns/op
Benchmark_Mutation-4             	  300000	      4155 ns/op
Benchmark_directors_parallel-4   	  200000	      6966 ns/op
Benchmark_Movies_parallel-4      	  200000	      6151 ns/op
Benchmark_Filters_parallel-4     	  300000	      5015 ns/op
Benchmark_Geq_parallel-4         	  300000	      4485 ns/op
Benchmark_Date_parallel-4        	  300000	      4515 ns/op
Benchmark_Mutation_parallel-4    	 1000000	      2161 ns/op


Using Channel (old):
--------------
Benchmark_directors-4            	  100000	     18663 ns/op
Benchmark_Movies-4               	  100000	     16097 ns/op
Benchmark_Filters-4              	  100000	     14007 ns/op
Benchmark_Geq-4                  	  200000	     11701 ns/op
Benchmark_Date-4                 	  200000	     11687 ns/op
Benchmark_Mutation-4             	  300000	      5167 ns/op
Benchmark_directors_parallel-4   	  200000	      8766 ns/op
Benchmark_Movies_parallel-4      	  200000	      7537 ns/op
Benchmark_Filters_parallel-4     	  200000	      6486 ns/op
Benchmark_Geq_parallel-4         	  300000	      5390 ns/op
Benchmark_Date_parallel-4        	  200000	      5462 ns/op
Benchmark_Mutation_parallel-4    	 1000000	      2326 ns/op
