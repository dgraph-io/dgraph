# Code Clarity and Readability Rules

## Overview

All code, especially test code, must prioritize clarity and readability over brevity. Code is read
far more often than it's written, so optimize for the reader's understanding.

## Core Principles

### 1. Clarity Over Cleverness

- Write code that a junior developer can understand in 6 months
- If there's a choice between clever/short code and clear/longer code, always choose clear
- Avoid one-liners that pack multiple operations unless they're trivial

### 2. Test Code Readability Standards

Test code has even higher readability requirements than production code:

- **Descriptive Variable Names**: Use full, descriptive names instead of abbreviations

  - Bad: `n`, `i`, `l`, `r`
  - Good: `heapSize`, `parentIndex`, `leftChildIndex`, `rightChildIndex`

- **Clear Function Names**: Test helper functions should describe exactly what they do

  - Bad: `check()`, `validate()`
  - Good: `validateHeapPropertyMaintained()`, `assertNodeHasCorrectChildren()`

- **Logical Separation**: Break complex operations into clearly named steps
  - Use intermediate variables to make calculations explicit
  - Separate setup, action, and assertion phases

### 3. Comment Requirements

**Mandatory Comments For:**

- Complex algorithms or mathematical operations
- Non-obvious business logic
- Performance-critical sections
- Workarounds or bug fixes
- Any code that made you think "hmm, how does this work?"

**Comment Structure:**

```go
// Why: Explain the purpose/reasoning
// What: Describe what the code does (if not obvious)
// How: Explain the approach for complex logic
```

### 4. Function Structure Standards

**Test Helper Functions Must:**

- Have a single, clear responsibility
- Include parameter validation with meaningful error messages
- Use descriptive error messages that help debug failures
- Separate concerns into logical blocks with whitespace

**Example of Good Structure:**

```go
// validateHeapPropertyMaintained verifies that the min-heap property is satisfied:
// each parent node's value must be <= both of its children's values
func validateHeapPropertyMaintained(t *testing.T, heap *uint64Heap) {
    heapSize := len(*heap)

    // Check each parent node against its children
    for parentIndex := 0; parentIndex < heapSize; parentIndex++ {
        leftChildIndex := 2*parentIndex + 1
        rightChildIndex := 2*parentIndex + 2

        // Validate left child relationship
        if leftChildIndex < heapSize {
            parentValue := (*heap)[parentIndex].val
            leftChildValue := (*heap)[leftChildIndex].val

            require.True(t, parentValue <= leftChildValue,
                "Min-heap property violated: parent node %d has value %d but left child node %d has smaller value %d",
                parentIndex, parentValue, leftChildIndex, leftChildValue)
        }

        // Validate right child relationship
        if rightChildIndex < heapSize {
            parentValue := (*heap)[parentIndex].val
            rightChildValue := (*heap)[rightChildIndex].val

            require.True(t, parentValue <= rightChildValue,
                "Min-heap property violated: parent node %d has value %d but right child node %d has smaller value %d",
                parentIndex, parentValue, rightChildIndex, rightChildValue)
        }
    }
}
```

### 5. Error Message Standards

Error messages in tests must be:

- **Specific**: Include actual values, indices, and context
- **Actionable**: Help the reader understand what went wrong
- **Consistent**: Use the same format across similar assertions

**Bad Error Messages:**

- "Heap property violated"
- "Values don't match"
- "Invalid state"

**Good Error Messages:**

- "Min-heap property violated: parent node 5 has value 100 but left child node 11 has smaller value
  50"
- "Expected user ID 12345 but found 67890 in database record"
- "Queue should be empty after processing all items, but contains 3 remaining items: [item1, item2,
  item3]"

### 6. Code Organization in Tests

**Structure test functions as:**

1. **Setup**: Prepare test data and dependencies
2. **Action**: Execute the code under test
3. **Assertion**: Verify the results
4. **Cleanup**: Clean up resources (if needed)

Use whitespace and comments to separate these phases clearly.

### 7. Enforcement

**Before writing any code, ask:**

- Would a new team member understand this in 30 seconds?
- Are the variable names self-documenting?
- Do the error messages help me debug problems?
- Is the algorithm or approach obvious from reading the code?

**If the answer to any is "no", refactor for clarity.**

## Examples to Avoid

### Don't Do This:

```go
// Confusing variable names, unclear purpose, poor error messages
func check(t *testing.T, h *uint64Heap) {
    for i := 0; i < len(*h); i++ {
        if l := 2*i+1; l < len(*h) && (*h)[i].val > (*h)[l].val {
            require.Fail(t, "bad heap")
        }
        if r := 2*i+2; r < len(*h) && (*h)[i].val > (*h)[r].val {
            require.Fail(t, "bad heap")
        }
    }
}
```

### Do This Instead:

```go
// Clear purpose, descriptive names, helpful error messages
func validateMinHeapProperty(t *testing.T, heap *uint64Heap) {
    // A min-heap satisfies: parent <= both children for all nodes
    heapSize := len(*heap)

    for parentIndex := 0; parentIndex < heapSize; parentIndex++ {
        leftChildIndex := 2*parentIndex + 1
        rightChildIndex := 2*parentIndex + 2

        // Check left child if it exists
        if leftChildIndex < heapSize {
            assertParentNotGreaterThanChild(t, heap, parentIndex, leftChildIndex, "left")
        }

        // Check right child if it exists
        if rightChildIndex < heapSize {
            assertParentNotGreaterThanChild(t, heap, parentIndex, rightChildIndex, "right")
        }
    }
}

func assertParentNotGreaterThanChild(t *testing.T, heap *uint64Heap, parentIndex, childIndex int, childType string) {
    parentValue := (*heap)[parentIndex].val
    childValue := (*heap)[childIndex].val

    require.True(t, parentValue <= childValue,
        "Min-heap property violated: parent node %d (value=%d) is greater than %s child node %d (value=%d)",
        parentIndex, parentValue, childType, childIndex, childValue)
}
```

This approach makes the code self-documenting and much easier to debug when tests fail.
