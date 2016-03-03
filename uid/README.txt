Dropped 61 nodes (cum <= 6.05MB)
Showing top 10 nodes out of 52 (cum >= 149.99MB)
      flat  flat%   sum%        cum   cum%
  315.56MB 26.10% 26.10%   315.56MB 26.10%  github.com/dgraph-io/dgraph/posting.NewList
   87.51MB  7.24% 33.33%    87.51MB  7.24%  github.com/dgraph-io/dgraph/uid.stringKey
      80MB  6.62% 39.95%   105.50MB  8.72%  github.com/dgraph-io/dgraph/uid.(*lockManager).newOrExisting
   78.01MB  6.45% 46.40%    78.01MB  6.45%  github.com/dgraph-io/dgraph/posting.Key
   77.50MB  6.41% 52.81%      155MB 12.82%  github.com/zond/gotomic.(*Hash).getBucketByIndex
   77.50MB  6.41% 59.22%    77.50MB  6.41%  github.com/zond/gotomic.newMockEntry
   74.50MB  6.16% 65.38%    74.50MB  6.16%  github.com/zond/gotomic.newRealEntryWithHashCode
      51MB  4.22% 69.60%    74.01MB  6.12%  github.com/dgraph-io/dgraph/posting.(*List).merge
   48.50MB  4.01% 73.61%    48.50MB  4.01%  github.com/dgraph-io/dgraph/loader.(*state).readLines
   43.50MB  3.60% 77.21%   149.99MB 12.40%  github.com/zond/gotomic.(*Hash).PutIfMissing
(pprof) list uid.stringKey
Total: 1.18GB
ROUTINE ======================== github.com/dgraph-io/dgraph/uid.stringKey in /home/manishrjain/go/src/github.com/dgraph-io/dgraph/uid/assigner.go
   87.51MB    87.51MB (flat, cum)  7.24% of Total
         .          .    186:   rerr := pl.AddMutation(t, posting.Set)
         .          .    187:   return uid, rerr
         .          .    188:}
         .          .    189:
         .          .    190:func stringKey(xid string) []byte {
   87.51MB    87.51MB    191:   var buf bytes.Buffer
         .          .    192:   buf.WriteString("_uid_|")
         .          .    193:   buf.WriteString(xid)
         .          .    194:   return buf.Bytes()
         .          .    195:}
         .          .    196:

After changing the code to return []byte("_uid_" + xid), the memory profiler no longer shows it.

$ go tool pprof uidassigner mem.prof 
Entering interactive mode (type "help" for commands)
(pprof) top10
907.59MB of 1139.29MB total (79.66%)
Dropped 86 nodes (cum <= 5.70MB)
Showing top 10 nodes out of 48 (cum >= 45.01MB)
      flat  flat%   sum%        cum   cum%
  310.56MB 27.26% 27.26%   310.56MB 27.26%  github.com/dgraph-io/dgraph/posting.NewList
      89MB  7.81% 35.07%       89MB  7.81%  github.com/zond/gotomic.newMockEntry
   81.50MB  7.15% 42.23%   170.51MB 14.97%  github.com/zond/gotomic.(*Hash).getBucketByIndex
   81.50MB  7.15% 49.38%      109MB  9.57%  github.com/dgraph-io/dgraph/uid.(*lockManager).newOrExisting
   76.51MB  6.72% 56.09%    76.51MB  6.72%  github.com/dgraph-io/dgraph/posting.Key
   72.50MB  6.36% 62.46%    72.50MB  6.36%  github.com/zond/gotomic.newRealEntryWithHashCode
   55.50MB  4.87% 67.33%    63.50MB  5.57%  github.com/dgraph-io/dgraph/posting.(*List).merge
      50MB  4.39% 71.72%       50MB  4.39%  github.com/dgraph-io/dgraph/loader.(*state).readLines
   45.50MB  3.99% 75.71%   150.52MB 13.21%  github.com/zond/gotomic.(*Hash).PutIfMissing
   45.01MB  3.95% 79.66%    45.01MB  3.95%  github.com/google/flatbuffers/go.(*Builder).growByteBuffer

