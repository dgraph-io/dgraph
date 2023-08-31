How to generate a unique list of uids by querying list of posting lists?

Sol 1:
- Say there're k posting lists involved.
- One way to do so is to have a heap of k elements.
- At each iteration, we pop() an element from the heap (log k)
- Advance the pointer of that posting list, and retrieve another element (involves mutex read lock)
- Push() that element into the heap (log k)
- This would give us O(N*log k), with mutex lock acquired N times.
- With N=1000 and k=5, this gives us 1000 * ln(5) ~ 1600

Performance Improvements (memory tradeoff) [Sol1a]:
- We can alleviate the need for mutex locks by copying over all the posting list uids in separate vectors.
- This would avoid N lock acquisitions, only requiring the best-case scenario of k locks.
- But this also means all the posting list uids would be stored in memory.

Performance with Memory [Sol1b]:
- Use k channels, with each channel only maintaining a buffer of say 1000 uids.
- In fact, keep the read lock acquired during this process, to avoid the posting list from changing during a query.
- So, basically have a way for a posting list to stream uids to a blocking channel, after having acquired a read lock.
- Overall this process of merging uids shouldn't take that long anyways; so this won't starve writes, only delay them.

Another way [Sol2]:
- Pick a posting list, copy all it's uids in one go (one mutex lock)
- Use a binary tree to store uids. Eliminate duplicates.
- Iterate over each element in the uids vector, and insert into binary tree. [O(log N) max per insert]
- Repeat with other posting lists.
- This would give us O(N log N) complexity, with mutex lock acquired k times.
- With N=1000 and k=5, this gives us 1000 * ln(1000) ~ 7000
- Not choosing this path.

Solution: Sol1b
