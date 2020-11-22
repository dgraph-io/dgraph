# Group Delete Test

This test runs a scenario where nodes are removed from groups to make zero delete those
groups. At every stage we check the zero state to make sure the nodes and groups are deleted.

Process:

1. Bring up a cluster with 3 groups, 1 node each.
2. Delete node from group 3
3. Check that zero deleted group 3
4. Run a query to test that the cluster is viable
5. Delete node from group 2
6. Check that zero deleted group 2
7. Run a query to test that the cluster is viable
