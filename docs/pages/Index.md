---
layout: default
permalink: /
---

<div align="center">
  <img alt="Gossamer logo" style="opacity: 0.9" src="https://chainsafe.github.io/gossamer/assets/img/gossamer_banner_white.png" width="500" />
</div>
<div align="center">
  <h1 class="title">Gossamer Docs | <i>The Official Documentation for Gossamer</i></h1>
</div>
<br/>

## Welcome

***Gossamer*** is an implementation of the [Polkadot Host](https://github.com/w3f/polkadot-spec) - a blockchain framework used to build and run nodes for different blockchain protocols within the Polkadot ecosystem.

Gossamer includes node implementations for major blockchains within the Polkadot ecosystem and makes building node implementations for other blockchains trivial; blockchains built with [Substrate](https://github.com/paritytech/substrate) can plug their compiled runtime into Gossamer to create a node implementation in Go.

***Gossamer Docs*** is an evolving set of documents and resources to help you understand Gossamer, the Polkadot ecosystem, and how to build and run nodes using Gossamer. 

- If you are new to Gossamer and the Polkadot ecosystem, we recommend starting with [this video](https://www.youtube.com/watch?v=nYkbYhM5Yfk) and then working your way through [General Resources](./general-resources/).

- If you are already familiar with Gossamer and the Polkadot ecosystem, or you just want to dive in, head over to [Get Started](./get-started) to run your first node using Gossamer.

- If you are looking to build a node with Gossamer, learn how Gossamer can be used to build and run custom node implementations using Gossamer as a framework (keep reading).

## Framework

Gossamer is a ***modular blockchain framework*** used to build and run nodes for different blockchain protocols within the Polkadot ecosystem.

- The ***simplest*** way to use the framework is using the base node implementation with a custom configuration file (see [Configuration](./configuration/)).

- The ***more advanced***  way to use the framework is using the base node implementation with a compiled runtime and custom runtime imports (see [Import Runtime](./import-runtime/)). 

- The ***most advanced***  way to use the framework is building custom node services or a custom node implementation (see [Custom Services](./custom-services/)).

Our primary focus has been an initial implementation of the Polkadot Host. Once we feel confident our initial implementation is fully operational and secure, we will expand the Gossamer framework to include a runtime library and other tools and services that will enable Go developers to build, test, and run custom-built blockchain protocols within the Polkadot ecosystem.

## Table of Contents

- **[Run Nodes](./run-nodes/)**
    - [Get Started](./get-started/)
    - [Command-Line](./command-line/)
    - [Official Nodes](./official-nodes/)

- **[Build Nodes](./build-nodes/)**
    - [Configuration](./configuration/)
    - [Import Runtime](./import-runtime/)
    - [Custom Services](./custom-services/)

- **[Implementation](./implementation/)**
    - [Package Library](./package-library/)
    - [Host Architecture](./host-architecture/)
    - [Integration Tests](./integration-tests/)

- **[Resources](./resources/)**
    - [General Resources](./general-resources/)
    - [Developer Resources](./developer-resources/)

<!--

- **[Appendix](./appendix/)**
    - [SCALE Examples](./scale-examples/)

-->

## Connect

Let us know if you have any feedback or ideas that might help us improve our documentation or if you have any resources that you would like to see added. If you are planning to use Gossamer or any of the Gossamer packages, please say hello! You can find us on [Discord](https://discord.gg/Xdc5xjE).

## Contribute

Contributions to this site and its contents are more than welcome. We built this site with [jekyll](https://jekyllrb.com/) - the site configuration and markdown files are within [gossamer/docs](https://github.com/ChainSafe/gossamer/tree/development/docs). If you would like to contribute, please read [Code of Conduct](https://github.com/ChainSafe/gossamer/blob/development/.github/CODE_OF_CONDUCT.md) and [Contributing Guidelines](https://github.com/ChainSafe/gossamer/blob/development/.github/CONTRIBUTING.md) before getting started.
