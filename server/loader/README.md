### 17 May 2016

#### Loader CPU and MEM performance

```
(pprof) top15
2210.85s of 2894.10s total (76.39%)
Dropped 520 nodes (cum <= 14.47s)
Showing top 15 nodes out of 134 (cum >= 22.88s)
      flat  flat%   sum%        cum   cum%
   437.06s 15.10% 15.10%   1175.03s 40.60%  runtime.scanobject
   414.24s 14.31% 29.42%    414.25s 14.31%  runtime.heapBitsForObject
   413.74s 14.30% 43.71%    424.87s 14.68%  runtime.cgocall
   300.81s 10.39% 54.10%    329.67s 11.39%  runtime.greyobject
   180.74s  6.25% 60.35%    328.67s 11.36%  github.com/dgraph-io/dgraph/posting.(*List).get
   101.91s  3.52% 63.87%    102.63s  3.55%  github.com/dgraph-io/dgraph/vendor/github.com/zond/gotomic.(*element).isDeleted
    68.48s  2.37% 66.24%     68.48s  2.37%  runtime.(*mspan).sweep.func1
    66.74s  2.31% 68.54%    980.63s 33.88%  runtime.mallocgc
    61.63s  2.13% 70.67%     74.88s  2.59%  github.com/dgraph-io/dgraph/posting.(*List).mindexInsertAt
    38.64s  1.34% 72.01%     38.71s  1.34%  runtime.heapBitsSetType
    30.41s  1.05% 73.06%     65.79s  2.27%  github.com/dgraph-io/dgraph/vendor/github.com/zond/gotomic.(*Hash).getBucketByIndex
    26.40s  0.91% 73.97%     95.10s  3.29%  runtime.heapBitsSweepSpan
    24.05s  0.83% 74.80%     24.05s  0.83%  runtime.memmove
    23.12s   0.8% 75.60%    124.44s  4.30%  runtime.(*mspan).sweep
    22.88s  0.79% 76.39%     22.88s  0.79%  runtime/internal/atomic.Or8
```

```
3102MB of 3278.34MB total (94.62%)
Dropped 89 nodes (cum <= 16.39MB)
Showing top 15 nodes out of 50 (cum >= 277.14MB)
      flat  flat%   sum%        cum   cum%
  817.16MB 24.93% 24.93%   817.16MB 24.93%  github.com/dgraph-io/dgraph/posting.NewList
  437.51MB 13.35% 38.27%   437.51MB 13.35%  github.com/dgraph-io/dgraph/vendor/github.com/zond/gotomic.newMockEntry
  429.01MB 13.09% 51.36%   866.53MB 26.43%  github.com/dgraph-io/dgraph/vendor/github.com/zond/gotomic.(*Hash).getBucketByIndex
  213.51MB  6.51% 57.87%   213.51MB  6.51%  github.com/dgraph-io/dgraph/vendor/github.com/zond/gotomic.newRealEntryWithHashCode
  203.52MB  6.21% 64.08%   203.52MB  6.21%  github.com/dgraph-io/dgraph/store/rocksdb._Cfunc_GoBytes
  155.01MB  4.73% 68.81%   490.53MB 14.96%  github.com/dgraph-io/dgraph/posting.(*List).init
  145.24MB  4.43% 73.24%   145.24MB  4.43%  github.com/dgraph-io/dgraph/vendor/github.com/zond/gotomic.(*Hash).grow
  144.50MB  4.41% 77.65%   462.13MB 14.10%  github.com/dgraph-io/dgraph/vendor/github.com/zond/gotomic.(*Hash).PutIfMissing
  102.01MB  3.11% 80.76%   102.01MB  3.11%  bytes.makeSlice
   92.50MB  2.82% 83.58%   335.53MB 10.23%  github.com/dgraph-io/dgraph/posting.(*List).getPostingList
   89.51MB  2.73% 86.31%    89.51MB  2.73%  github.com/dgraph-io/dgraph/vendor/github.com/google/flatbuffers/go.(*Builder).growByteBuffer
      83MB  2.53% 88.84%       83MB  2.53%  github.com/dgraph-io/dgraph/vendor/github.com/zond/gotomic.(*element).add
      81MB  2.47% 91.31%   113.01MB  3.45%  github.com/dgraph-io/dgraph/posting.(*List).merge
      72MB  2.20% 93.51%  2506.85MB 76.47%  github.com/dgraph-io/dgraph/posting.GetOrCreate
   36.50MB  1.11% 94.62%   277.14MB  8.45%  github.com/dgraph-io/dgraph/posting.(*List).AddMutation
```
