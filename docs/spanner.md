# Notes from Spanner

- Globally meaningful commit transactions.
- In correct serialization order
- If can't assign monotonically increasing timestamps, then slows down the system.
I'd guess that means writes, because read only operations don't get any txns.
- Assigns timestamps to data.

### Architecture

- Zonemaster assigns data to 100 - 1K
- Spanserver serves data to clients via 100 - 1K
- Tablets
- Spanserver also has a lock table for 2 phase locking.
Maps *range of keys --> Lock states*.
- Each tablet has a Paxos state machine, also stores logs
- Each Paxos state machine contains metadata + logs
- Hence, logs are stored twice.
- Other tablets would contain the same data as replicas. Leader via Paxos.
- Each leader would contain transaction manager.
- This leader + replicas config is called a group.
- Each group would participate with other groups in case of a transaction
involving multiple range of keys.
- **In case of Dgraph, every txn would involve other groups**
- Each txn manager would co-ordinate with other txn managers, by participating
in paxos leader election.
- Txn manager acquires locks.
- In one Paxos group, one Participant Leader, other would be Participant slaves.
- Among participating paxos groups, one would be coordinator.
- Participant leader of that group = coordinator leader.
- Participant slaves of that group = coordinator slaves.

### Reads
- Ts is system-chosen without locking, so no incoming writes are blocked.

### 2 Phase Locking
- Read and write locks
- Expanding phase: Locks are only acquired
- Shrinking phase: Locks are only released
- S2PL **Strict 2 Phase Locking**: Release its write locks only after it
has ended: committed or aborted.
- SS2PL **Strong S2PL**: Release both write and read locks only after txn has ended.
- Aka, *release all locks only after txn end*

### Deadlock Prevention [link](http://www.cs.colostate.edu/~cs551/CourseNotes/Deadlock/WaitWoundDie.html)
- **Wait Die**: If am older than lock holder, wait. If am younger, die.
Would cause younger process to die again and again.
- **Wound-Wait**: If am older than lock holder, preempt holder. If am younger, wait.
Better than *Wait-Die*.
- To avoid **starvation**, don't assign new ts each time it restarts.

### Read Write Transactions (Theory)
- 2 Phase Locking
- Wound-Wait
- Spanner ts = ts of Paxos write of txn commit record.
- Within each Paxos group, writes in monotonically increasing order, even across leaders.
- Single leader replica can easily assign monotonically increasing ts (duh!)
- Across leaders, a leader must only assign ts within the interval of its leader lease (??)
- When ts s is assigned, smax = s, to preserve disjointness.

```
ei,start,  ei,commit -> start and commit events
si = commit ts of txn Ti.
tabs(e2,start) > tabs(e1,commit) => s2 > s1
```

- **START**: Coordinator leader assigns a commit ts si > TT.now().latest,
computed after ei,server = arrival event for commit request at self.
- **Commit Wait**: Client can't see any data by Ti, until TT.after(si).
```
s1 < tabs(e1,commit) // commit wait
tabs(e1,commit) < tabs(e2,start) // scenario
tabs(e2,start) < tabs(e2,server)  // causality
tabs(e2,server) < s2  // start
=> s1 < s2 *oh yeah*
```

### Read Write Transactions
- Reads come back with timestamps. Writes are buffered in client.
- Client sends keep-alive messages to participant leaders.
- Begin 2 phase commit after completing all reads, and buffering all writes.
- Chose a coordinator group and send commit message to each participant leader.
- Client drives 2 phase commit.
- Non-coordinator Participant leader acquires write locks.
- Prepare ts > previously assigned txn ts.
- Logs a prepare record via Paxos.
- Each participant notifies coordinator of the prepare ts.
- Coordinator leader also acquires write locks.
- Commit ts >= all prepare ts of participant leaders.
- Commit ts > TT.now().latest at the time commit message was received.
- Commit ts > previous assigned txn ts.
- Log commit record via Paxos.
- Wait until TT.after(s), with expected wait of 2e, e = margin of error.
- Send commit ts to client and all participant leaders.
- Each leader logs txn outcome via Paxos.
- All apply at the same timestamp, and release locks.
