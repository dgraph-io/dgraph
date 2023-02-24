```yaml
  RFC: PREFIX + NUMBER # Choose a unique number for your RFC.
  Title: RFC TITLE
  Author: NAME <EMAIL> # It can be Discuss user or any public email
  Co-Author: NAME <EMAIL> # It can be Discuss user or any public email and you can add more lines with co-authors.
  Status: STATUS # e.g., "Accepted," "Rejected"
  Type: TYPE # e.g., "Design," "Process," "Informational")**
  Model: Model # e.g., "RFC," "Deprecation," "Post-mortem")
  Created: DATE # Not necessarily the day of publication
  Replaces: RFC NUMBER # Rejected, depreacted RFC or other.
  Superseded-By: NUMBER # It can be left blank if the RFC has not been superseded.
```

## Title: RFC + number: [Title of RFC]

### Summary:

A brief summary of the RFC. Provide a brief overview of the main topics covered in the document.

### Motivation:

An explanation of the need for the RFC and the problem it aims to solve.

### Abstract Design:

If necessary, a high-level description of the design of the solution proposed in the RFC. This section may include
pseudocode to clarify concepts.

### Technical Details:

A detailed explanation of the technical implementation of the solution proposed in the RFC.

### Drawbacks:

An examination of the limitations and potential negative consequences of the solution proposed in the RFC.
Such as impact in third parties tools, impact in the core, backwards Compatibility and so on.

### Alternatives:

An exploration of other potential solutions to the problem addressed by the RFC.

### Adoption Strategy:

A plan for how the proposed solution in the RFC will be adopted and integrated into the Dgraph Core.

--------

Explanation(Delete this in your RFC):

The "Type" field in an RFC template is used to categorize the type of proposal that the RFC represents. In the context
of Dgraph, it can be used to indicate the nature or purpose of the proposal.

Here are some examples of types that might be used in an RFC for Dgraph:

* Design: This type is used when the RFC proposes a new design or architecture for a feature or system in Dgraph products.

* Process: This type is used when the RFC proposes a new process or workflow for the development of Dgraph products.

* Informational: This type is used when the RFC provides information that is important to the development of Dgraph, but does not propose any changes to the design or process.
