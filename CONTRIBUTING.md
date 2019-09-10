# Contribution Guidelines

Thanks for checking out our Polkadot Runtime Implementation! We're excited to hear and learn from you.

We've put together the following guidelines to help you figure out where you can best be helpful. The Web3 foundation has a comprehensive collection of [Polkadot Resources](https://github.com/w3f/web3/blob/537a2518c24e96b05ceadd9f31348669e72b8841/docs/layer_1/platforms/polkadot.md) for both part-time and core contributors to the project in order to get up to speed.

Additionally, the [Polkadot Runtime Specification](https://research.web3.foundation/en/latest/web/viewer.html?file=../pdf/polkadot_re_spec.pdf) serves as the primary specification, however it is currently in its draft status so things may be subject to change.

Feel free to fork our repo and start creating PR’s after assigning yourself to an issue of interest.

## Contribution Steps

**1. Set up gossamer following the instructions in README.md.**

**2. Fork the gossamer repo.**

**3. Create a local clone of gossamer.**

**4. Link your local clone to the fork on your Github repo.**

```
$ git remote add your-gossamer-repo https://github.com/<your_github_user_name>/gossamer.git
```

**5. Link your local clone to the ChainSafe Systems repo so that you can easily fetch future changes to the ChainSafe Systems repo.**

```
$ git remote add gossamer https://github.com/ChainSafe/gossamer.git
$ git remote -v (you should see myrepo and gossamer in the list of remotes)
```

**6. Find an issue to work on.**

Check out [open issues](https://github.com/ChainSafe/gossamer/issues) and pick one. Leave a comment to let the development team know that you would like to work on it. Or examine the code for areas that can be improved and leave an issue to state that you would like to work on it.

**7. Make improvements to the code.**

Each time you work on the code be sure that you are working on the branch that you have created as opposed to your local copy of the ChainSafe Systems repo. Keeping your changes segregated in this branch will make it easier to merge your changes into the repo later.

```
$ git checkout feature-in-progress-branch
```

**8. Test your changes.**

Changes that only affect a single file can be tested with

```
$ go test <file_you_are_working_on>
```

**9. Lint your changes.**

Before opening a pull request be sure to run the linter

```
$ gometallinter ./...
```

**10. Create a pull request.**

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

Anyone can become a part-time contributor and help out on implementing gossamer. Contributions can be made in the following ways:

-   Engaging in Gitter conversations, asking questions on how to begin contributing to the project
-   Opening up github issues to express interest in code to implement
-   Opening up PRs referencing any open issue in the repo. PRs should include:
    -   Detailed context of what would be required for merge
    -   Tests that are consistent with how other tests are written in our implementation
-   Proper labels, milestones, and projects (see other closed PRs for reference)
-   Follow up on open PRs
    -   Have an estimated timeframe to completion and let the core contributors know if a PR will take longer than expected

We do not expect all part-time contributors to be experts on all the latest Polkadot documentation, but all contributors should at least be familiarized with the Polkadot Specification fundamentals.

### Core Contributors

Core contributors are currently comprised of members of the ChainSafe Systems team. Core devs have all of the responsibilities of part-time contributors plus the majority of the following:

-   Stay up to date on the latest Polkadot research and updates
- 	Commit high quality code on core functionality
-   Monitor github issues and PR’s to make sure owner, labels, descriptions are correct
-   Formulate independent ideas, suggest new work to do, point out improvements to existing approaches
-   Participate in code review, ensure code quality is excellent, and ensure high code coverage
