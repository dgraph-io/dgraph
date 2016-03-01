go tool pprof --alloc_objects uidassigner heap.prof

(pprof) top10
196427053 of 207887723 total (94.49%)
Dropped 41 nodes (cum <= 1039438)
Showing top 10 nodes out of 31 (cum >= 8566234)
      flat  flat%   sum%        cum   cum%
  55529704 26.71% 26.71%   55529704 26.71%  github.com/dgraph-io/dgraph/rdf.Parse
  28255068 13.59% 40.30%   30647245 14.74%  github.com/dgraph-io/dgraph/posting.(*List).getPostingList
  20406729  9.82% 50.12%   20406729  9.82%  github.com/zond/gotomic.newRealEntryWithHashCode
  17777182  8.55% 58.67%   17777182  8.55%  strings.makeCutsetFunc
  17582839  8.46% 67.13%   17706815  8.52%  github.com/dgraph-io/dgraph/loader.(*state).readLines
  15139047  7.28% 74.41%   88445933 42.55%  github.com/dgraph-io/dgraph/loader.(*state).parseStream
  12927366  6.22% 80.63%   12927366  6.22%  github.com/zond/gotomic.(*element).search
  10789028  5.19% 85.82%   66411362 31.95%  github.com/dgraph-io/dgraph/posting.GetOrCreate
   9453856  4.55% 90.37%    9453856  4.55%  github.com/zond/gotomic.(*hashHit).search
   8566234  4.12% 94.49%    8566234  4.12%  github.com/dgraph-io/dgraph/uid.stringKey


(pprof) list rdf.Parse
Total: 207887723
ROUTINE ======================== github.com/dgraph-io/dgraph/rdf.Parse in /home/mrjn/go/src/github.com/dgraph-io/dgraph/rdf/parse.go
  55529704   55529704 (flat, cum) 26.71% of Total
         .          .    118:	}
         .          .    119:	return val[1 : len(val)-1]
         .          .    120:}
         .          .    121:
         .          .    122:func Parse(line string) (rnq NQuad, rerr error) {
  54857942   54857942    123:	l := lex.NewLexer(line)
         .          .    124:	go run(l)
         .          .    125:	var oval string
         .          .    126:	var vend bool


This showed that lex.NewLexer(..) was pretty expensive in terms of memory allocation.
So, let's use sync.Pool here.

After using sync.Pool, this is the output:

422808936 of 560381333 total (75.45%)
Dropped 63 nodes (cum <= 2801906)
Showing top 10 nodes out of 62 (cum >= 18180150)
      flat  flat%   sum%        cum   cum%
 103445194 18.46% 18.46%  103445194 18.46%  github.com/Sirupsen/logrus.(*Entry).WithFields
  65448918 11.68% 30.14%  163184489 29.12%  github.com/Sirupsen/logrus.(*Entry).WithField
  48366300  8.63% 38.77%  203838187 36.37%  github.com/dgraph-io/dgraph/posting.(*List).get
  39789719  7.10% 45.87%   49276181  8.79%  github.com/dgraph-io/dgraph/posting.(*List).getPostingList
  36642638  6.54% 52.41%   36642638  6.54%  github.com/dgraph-io/dgraph/lex.NewLexer
  35190301  6.28% 58.69%   35190301  6.28%  github.com/google/flatbuffers/go.(*Builder).growByteBuffer
  31392455  5.60% 64.29%   31392455  5.60%  github.com/zond/gotomic.newRealEntryWithHashCode
  25895676  4.62% 68.91%   25895676  4.62%  github.com/zond/gotomic.(*element).search
  18546971  3.31% 72.22%   72863016 13.00%  github.com/dgraph-io/dgraph/loader.(*state).parseStream
  18090764  3.23% 75.45%   18180150  3.24%  github.com/dgraph-io/dgraph/loader.(*state).readLines
