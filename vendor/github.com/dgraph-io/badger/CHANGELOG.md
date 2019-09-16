# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Serialization Versioning](VERSIONING.md).

## [Unreleased]

## [1.6.0] - 2019-07-01

This is a release including almost 200 commits, so expect many changes - some of them
not backward compatible.

Regarding backward compatibility in Badger versions, you might be interested on reading
[VERSIONING.md](VERSIONING.md).

_Note_: The hashes in parentheses correspond to the commits that impacted the given feature.

### New APIs

- badger.DB
  - DropPrefix (291295e)
  - Flatten (7e41bba)
  - KeySplits (4751ef1)
  - MaxBatchCount (b65e2a3)
  - MaxBatchSize (b65e2a3)
  - PrintKeyValueHistogram (fd59907)
  - Subscribe (26128a7)
  - Sync (851e462)

- badger.DefaultOptions() and badger.LSMOnlyOptions() (91ce687)
  - badger.Options.WithX methods

- badger.Entry (e9447c9)
  - NewEntry
  - WithMeta
  - WithDiscard
  - WithTTL 

- badger.Item
  - KeySize (fd59907)
  - ValueSize (5242a99)

- badger.IteratorOptions
  - PickTable (7d46029, 49a49e3)
  - Prefix (7d46029)

- badger.Logger (fbb2778)

- badger.Options
  - CompactL0OnClose (7e41bba)
  - Logger (3f66663)
  - LogRotatesToFlush (2237832)

- badger.Stream (14cbd89, 3258067)
- badger.StreamWriter (7116e16)
- badger.TableInfo.KeyCount (fd59907)
- badger.TableManifest (2017987)
- badger.Tx.NewKeyIterator (49a49e3)
- badger.WriteBatch (6daccf9, 7e78e80)

### Modified APIs

#### Breaking changes:

- badger.DefaultOptions and badger.LSMOnlyOptions are now functions rather than variables (91ce687)
- badger.Item.Value now receives a function that returns an error (439fd46)
- badger.Txn.Commit doesn't receive any params now (6daccf9)
- badger.DB.Tables now receives a boolean (76b5341)

#### Not breaking changes:

- badger.LSMOptions changed values (799c33f)
- badger.DB.NewIterator now allows multiple iterators per RO txn (41d9656)
- badger.Options.TableLoadingMode's new default is options.MemoryMap (6b97bac)

### Removed APIs

- badger.ManagedDB (d22c0e8)
- badger.Options.DoNotCompact (7e41bba)
- badger.Txn.SetWithX (e9447c9)

### Tools:

- badger bank disect (13db058)
- badger bank test (13db058) --mmap (03870e3)
- badger fill (7e41bba)
- badger flatten (7e41bba)
- badger info --histogram (fd59907) --history --lookup --show-keys --show-meta --with-prefix (09e9b63) --show-internal (fb2eed9)
- badger benchmark read (239041e)
- badger benchmark write (6d3b67d)

## [1.5.5] - 2019-06-20

* Introduce support for Go Modules

## [1.5.3] - 2018-07-11
Bug Fixes:
* Fix a panic caused due to item.vptr not copying over vs.Value, when looking
    for a move key.

## [1.5.2] - 2018-06-19
Bug Fixes:
* Fix the way move key gets generated.
* If a transaction has unclosed, or multiple iterators running simultaneously,
    throw a panic. Every iterator must be properly closed. At any point in time,
    only one iterator per transaction can be running. This is to avoid bugs in a
    transaction data structure which is thread unsafe.

* *Warning: This change might cause panics in user code. Fix is to properly
    close your iterators, and only have one running at a time per transaction.*

## [1.5.1] - 2018-06-04
Bug Fixes:
* Fix for infinite yieldItemValue recursion. #503
* Fix recursive addition of `badgerMove` prefix. https://github.com/dgraph-io/badger/commit/2e3a32f0ccac3066fb4206b28deb39c210c5266f
* Use file size based window size for sampling, instead of fixing it to 10MB. #501

Cleanup:
* Clarify comments and documentation.
* Move badger tool one directory level up.

## [1.5.0] - 2018-05-08
* Introduce `NumVersionsToKeep` option. This option is used to discard many
  versions of the same key, which saves space.
* Add a new `SetWithDiscard` method, which would indicate that all the older
  versions of the key are now invalid. Those versions would be discarded during
  compactions.
* Value log GC moves are now bound to another keyspace to ensure latest versions
  of data are always at the top in LSM tree.
* Introduce `ValueLogMaxEntries` to restrict the number of key-value pairs per
  value log file. This helps bound the time it takes to garbage collect one
  file.

## [1.4.0] - 2018-05-04
* Make mmap-ing of value log optional.
* Run GC multiple times, based on recorded discard statistics.
* Add MergeOperator.
* Force compact L0 on clsoe (#439).
* Add truncate option to warn about data loss (#452).
* Discard key versions during compaction (#464).
* Introduce new `LSMOnlyOptions`, to make Badger act like a typical LSM based DB.

Bug fix:
* (Temporary) Check max version across all tables in Get (removed in next
  release).
* Update commit and read ts while loading from backup.
* Ensure all transaction entries are part of the same value log file.
* On commit, run unlock callbacks before doing writes (#413).
* Wait for goroutines to finish before closing iterators (#421).

## [1.3.0] - 2017-12-12
* Add `DB.NextSequence()` method to generate monotonically increasing integer
  sequences.
* Add `DB.Size()` method to return the size of LSM and value log files.
* Tweaked mmap code to make Windows 32-bit builds work.
* Tweaked build tags on some files to make iOS builds work.
* Fix `DB.PurgeOlderVersions()` to not violate some constraints.

## [1.2.0] - 2017-11-30
* Expose a `Txn.SetEntry()` method to allow setting the key-value pair
  and all the metadata at the same time.

## [1.1.1] - 2017-11-28
* Fix bug where txn.Get was returing key deleted in same transaction.
* Fix race condition while decrementing reference in oracle.
* Update doneCommit in the callback for CommitAsync.
* Iterator see writes of current txn.

## [1.1.0] - 2017-11-13
* Create Badger directory if it does not exist when `badger.Open` is called.
* Added `Item.ValueCopy()` to avoid deadlocks in long-running iterations
* Fixed 64-bit alignment issues to make Badger run on Arm v7

## [1.0.1] - 2017-11-06
* Fix an uint16 overflow when resizing key slice

[Unreleased]: https://github.com/dgraph-io/badger/compare/v1.6.0...HEAD
[1.6.0]: https://github.com/dgraph-io/badger/compare/v1.5.5...v1.6.0
[1.5.5]: https://github.com/dgraph-io/badger/compare/v1.5.3...v1.5.5
[1.5.3]: https://github.com/dgraph-io/badger/compare/v1.5.2...v1.5.3
[1.5.2]: https://github.com/dgraph-io/badger/compare/v1.5.1...v1.5.2
[1.5.1]: https://github.com/dgraph-io/badger/compare/v1.5.0...v1.5.1
[1.5.0]: https://github.com/dgraph-io/badger/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/dgraph-io/badger/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/dgraph-io/badger/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/dgraph-io/badger/compare/v1.1.1...v1.2.0
[1.1.1]: https://github.com/dgraph-io/badger/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/dgraph-io/badger/compare/v1.0.1...v1.1.0
[1.0.1]: https://github.com/dgraph-io/badger/compare/v1.0.0...v1.0.1
