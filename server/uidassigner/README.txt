(pprof) top10
4.19mins of 7.46mins total (56.23%)
Dropped 569 nodes (cum <= 0.75mins)
Showing top 10 nodes out of 25 (cum >= 2.78mins)
      flat  flat%   sum%        cum   cum%
  1.71mins 22.86% 22.86%   4.03mins 54.02%  runtime.scanobject
  1.16mins 15.62% 38.48%   1.16mins 15.62%  runtime.heapBitsForObject
  0.95mins 12.77% 51.25%   1.21mins 16.16%  runtime.greyobject
  0.19mins  2.56% 53.81%   3.46mins 46.41%  runtime.mallocgc
  0.07mins  0.88% 54.69%   1.08mins 14.53%  runtime.mapassign1
  0.04mins  0.48% 55.17%   3.82mins 51.17%  runtime.systemstack
  0.03mins  0.35% 55.52%   0.92mins 12.36%  runtime.gcDrain
  0.02mins  0.26% 55.78%   1.42mins 18.98%  github.com/dgraph-io/dgraph/rdf.Parse
  0.02mins  0.23% 56.01%   0.92mins 12.29%  runtime.newobject
  0.02mins  0.22% 56.23%   2.78mins 37.25%  runtime.gcDrainN

The above CPU profile was generated before using any `sync.Pool`
