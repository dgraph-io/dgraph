# manual_tests

To run manual tests:

- Set `$DGRAPH_BIN` to the path of the Dgraph binary you want to test.
- Set `$EXIT_ON_FAILURE` to `1` to stop testing immediately after a test fails,
  leaving Dgraph running and the test directory intact.
- Execute `./test.sh`.

For long-running tests:

- These tests have been grouped under `testx::`, so they do not run by default.
- Execute `./test.sh testx::`

To add a new test:

- Create a function with the `test::` prefix.
- Return `0` on success, return `1` on failure.
