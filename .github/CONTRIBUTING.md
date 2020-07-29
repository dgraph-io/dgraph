# Contribution Guidelines

Thank you for your interest in our implementation of the Polkadot Runtime Environment Implementation! We're excited to get to know you and work with you on gossamer. We've put together these guidelines to help you figure out how you can help us.

At any point in this process feel free to reach out on [Discord](https://discord.gg/Xdc5xjE) with any questions or to say Hello :)

# Getting Started

Generally, it is important to have a basic understanding of Polkadot and the Polkadot Runtime Environment. Having a stronger  understanding will allow you to make more significant contributions. We've put together a list of resources that can help you develop this fundamental knowledge.  

The Web3 Foundation has a [Polkadot Wiki](https://wiki.polkadot.network/docs/en/learn-introduction) that would help both part-time and core contributors to the project in order to get up to speed. Our [wiki](https://github.com/ChainSafe/gossamer/wiki) also has some helpful resources. 

The [Polkadot Runtime Specification](https://research.web3.foundation/en/latest/_static/pdfview/viewer.html?file=../pdf/polkadot_re_spec.pdf) serves as our primary specification, however it is currently in its draft status so things may be subject to change.

One important thing distinction is that we are building the Polkadot Runtime Environment, not Polkadot itself. Given that, although a deep understanding of Polkadot is helpful, it's not critical to contribute to gossamer. To help understand how the Runtime Environment relates to Polkadot, check out this [talk that one of our team members gave at DotCon.](https://www.youtube.com/watch?v=nYkbYhM5Yfk)

## Contribution Steps

**1. Fork the gossamer repo.**

**2. Create a local clone of gossamer.**

```
go get -u github.com/ChainSafe/gossamer
cd $GOPATH/src/github.com/ChainSafe/gossamer
git init
```
You may encounter a `package github.com/ChainSafe/gossamer: no Go files in ...` message when doing `go get`. This is not an error, since there are no go files in the project root.

**3. Link your local clone to the fork on your Github repo.**

```
$ git remote add your-gossamer-repo https://github.com/<your_github_user_name>/gossamer.git
```

**4. Link your local clone to the ChainSafe Systems repo so that you can easily fetch future changes to the ChainSafe Systems repo.**
     
```
$ git remote add gossamer https://github.com/ChainSafe/gossamer.git
$ git remote -v (you should see myrepo and gossamer in the list of remotes)
```

**5. Find something to work on.**

To start, check out our open issues. We recommend starting with an [issue labeled `Good First Issue`](https://github.com/ChainSafe/gossamer/issues?q=is%3Aopen+is%3Aissue+label%3A%22Good+First+Issue%22). Leave a comment to let us know that you would like to work on it. 

Another option is to improve gossamer where you see fit based on your evaluation of our code. In order to best faciliate collabration, please create an issue before you start working on it.

**6. Make improvements to the code.**

Each time you work on the code be sure that you are working on the branch that you have created as opposed to your local copy of the gossamer repo. Keeping your changes segregated in this branch will make it easier to merge your changes into the repo later.

```
$ git checkout feature-in-progress-branch
```

**7. Test your changes.**

Changes that only affect a single file can be tested with

```
$ go test <file_you_are_working_on>
```

**8. Lint your changes.**

Before opening a pull request be sure to run the linter

```
$ gometallinter ./...
```

**9. Create a pull request.**

Navigate your browser to [https://github.com/ChainSafe/gossamer](https://github.com/ChainSafe/gossamer) and click on the new pull request button. In the “base” box on the left, change the branch to “**base development**”, the branch that you want your changes to be applied to. In the “compare” box on the right, select feature-in-progress-branch, the branch containing the changes you want to apply. You will then be asked to answer a few questions about your pull request. After you complete the questionnaire, the pull request will appear in the list of pull requests at [https://github.com/ChainSafe/gossamer/pulls](https://github.com/ChainSafe/gossamer/pulls).

## Note on memory intensive tests
Unfortunately, the free tier for CI's have a memory cap and some tests will cause the CI to experience an out of memory error.
In order to mitigate this we have introduced the concept of **short tests**. If your PR causes an out of memory error please seperate the tests into two groups
like below and make sure to label it `large`:

```
var stringTest = []string {
    "This causes no leaks"
}

var largeStringTest = []string {
    "Whoa this test is so big it causes an out of memory issue"
}

func TestStringTest(t *testing.T) {
    ...
}

func TestLargeStringTest(t *testing.T) {
   	if testing.Short() {
  		t.Skip("\033[33mSkipping memory intesive test for <TEST NAME> in short mode\033[0m")
    } else {
        ...
    }
}
```

## Contributor Responsibilities

We consider two types of contributions to our repo and categorize them as follows:

### Part-Time Contributors

Anyone can become a part-time contributor and help out on gossamer. Contributions can be made in the following ways:

-   Engaging in Discord conversations, asking questions on how to contribute to the project
-   Opening up Github issues to contribute ideas on how the code can be improved
-   Opening up PRs referencing any open issue in the repo. PRs should include:
    -   Detailed context of what would be required for merge
    -   Tests that are consistent with how other tests are written in our implementation
-   Proper labels, milestones, and projects (see other closed PRs for reference)
-   Follow up on open PRs
    -   Have an estimated timeframe to completion and let the core contributors know if a PR will take longer than expected

We do not expect all part-time contributors to be experts on all the latest Polkadot documentation, but all contributors should at least be familiarized with the fundamentals of the [Polkadot Runtime Specification](https://research.web3.foundation/en/latest/_static/pdfview/viewer.html?file=../pdf/polkadot_re_spec.pdf).

### Core Contributors

Core contributors are currently comprised of members of the ChainSafe Systems team. Core devs have all of the responsibilities of part-time contributors plus the majority of the following:

-   Participate in our software development process (standups, sprint planning, retrospectives, etc)
-   Stay up to date on the latest Polkadot research and updates
- 	Commit high quality code on core functionality
-   Monitor github issues and PR’s to make sure owner, labels, descriptions are correct
-   Formulate independent ideas, suggest new work to do, point out improvements to existing approaches
-   Participate in code review, ensure code quality is excellent, and ensure high code coverage
