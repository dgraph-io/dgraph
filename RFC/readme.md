# Welcome

### About this Directory

This is where we track all of our Request for Comments (RFCs) related to the Core products of Dgraph.

This directory provides a central and structured platform for discussing, documenting, and evaluating proposed changes
to the Dgraph Core. All stakeholders, including contributors, users, and members of the Dgraph community, are invited to
participate in the RFC process. Whether you have an idea for a new feature, a suggestion for improvement, or simply want
to stay informed about upcoming changes, this directory is the place to be.

Please note that while RFCs are primarily used for tracking major, complex, or potentially impactful changes, they are
not the only way to track changes in the directory. Other forms of tracking, such as issue tracking, are not ignored,
limited, or excluded. They can complement each other. RFCs serve as a way to focus attention and ensure that the
community has a clear understanding of the proposed changes and their potential impact.

## What is Dgraph?

Dgraph is an open-source, distributed graph database designed for fast and efficient querying and management of
connected data. It provides a scalable and flexible solution for managing and querying large amounts of data with
relationships. With its powerful graph query language, Dgraph allows developers to efficiently analyze and visualize
complex data relationships, making it ideal for a wide range of applications, including real-time recommendation systems,
social networks, and recommendation engines. With a focus on performance and ease of use, Dgraph is designed to help
organizations tackle complex data challenges and unlock the full potential of their connected data. Whether you are
working with big data, building a new application, or need a flexible solution for managing connected data, Dgraph is the
right choice for your needs.

# Scope of RFCs:

The purpose of this directory is to track and manage proposed changes to the core code of the main Dgraph repositories.
RFCs will not be used for documentation changes. Instead, they will focus on proposed changes to the application and
core code of the main repositories.

In addition to new feature proposals, we also track deprecated code through RFCs. When a proposed deprecation is opened,
an estimated time of completion (ETC) will be provided to determine whether the RFC will be implemented or not. This
process helps ensure that all stakeholders have a clear understanding of the changes being proposed and the timeline
for implementation.

We welcome contributions and feedback from the community on any proposed changes, and invite you to participate in the
discussions.

Here follows the RFC structure.

1. Request for Comments (RFCs) for Dgraph Core products. It will be located at "Proposals".
2. RFC for deprecation of code components. It will be located at "Deprecated".
3. Post-mortem analysis for resolved RFCs and deprecation processes. It will be located at "Post-mortem".

The RFC process provides a structured way to discuss and evaluate potential changes, while the deprecation process allows
us to provide an estimated timeline (ETA) for determining whether to continue with the process or cancel it based on
community interest. The Post-mortem directory will allow us to document and analyze the outcomes of resolved RFCs and
deprecation processes, which will help us improve our decision-making process in the future.

### How to start

The creation of an RFC in the Dgraph Core can start from a variety of sources. For example, an idea may arise from a
discussion on the Discourse forum or other community channels. If the proposal is considered to be valid, and contains a
logical or technical solution that can be implemented, it can be submitted as an RFC.

It is not necessary for the person submitting the proposal to discuss it beforehand. If they have sufficient knowledge
to propose an idea and the technical skills to understand the Core code, they can open a proposal. However, it is
encouraged to engage in discussion with the community to validate the idea and refine the proposal.

Once the proposal is submitted, it will go through a review process where the community and the core development team
can provide feedback and suggestions. The proposal may be revised and updated based on this feedback. Once the proposal
is deemed ready, it can be merged into the Dgraph Core codebase.


## Organizing RFCs

When contributing to the Dgraph RFC directory, it's important to follow a clear and organized structure to ensure that
all proposals and discussions are easily accessible and understandable.

In the top level of the RFC directy, there will be the "Proposals", "Deprecated", and "Post-mortem" directories.

When submitting a new RFC, you should create a new directory within the relevant product directory. For example, if you
are submitting an RFC for Dgraph Core, you should create a directory named "RFC-D0000" (where the numbers can be
adjusted to suit your proposal). You may choose to create your RFC in either .MD or .mediawiki format.

Additionally, you could include any supporting files, such as images or videos, within the same directory as your RFC.

Here's an overview of the file structure:

```lua
Dgraph RFC directory
|
|-- Proposals
|   |
|   |-- Dgraph
|   |   |
|   |   |-- RFC-D0000
|   |   |   |
|   |   |   |-- readme.md
|   |   |   |
|   |   |   |-- Supporting files (e.g. images, videos)
|
|-- Deprecated
|   |
|   |-- Dgraph
|   |   |
|   |   |-- RFC-D0000
|   |   |   |
|   |   |   |-- readme.md
|   |   |   |
|   |   |   |-- Supporting files
|
|-- Post-mortem
    |
    |-- Dgraph
    |   |
    |   |-- RFC-D0000
    |   |   |
    |   |   |-- readme.md
    |   |   |
    |   |   |-- Supporting files
    |

```
