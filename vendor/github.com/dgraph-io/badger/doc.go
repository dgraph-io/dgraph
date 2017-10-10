/*
Package badger implements an embeddable, simple and fast key-value store, written in pure Go.
It is designed to be highly performant for both reads and writes simultaneously.
Badger uses LSM tree, along with a value log, to separate keys from values, hence reducing
both write amplification and the size of the LSM tree.
This allows LSM tree to be served entirely from RAM, while the values are served from SSD.

Badger uses multiversion concurrency control, and supports transactions. It runs transactions
concurrently, with serializable snapshot isolation guarantees.
*/
package badger
