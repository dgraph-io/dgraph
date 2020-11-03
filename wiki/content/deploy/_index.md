+++
date = "2017-03-20T22:25:17+11:00"
title = "Deploy"
weight = 8
[menu.main]
  identifier = "deploy"
+++

<div class="landing">
  <div class="hero">
    <h1></h1>
    <p>
      This section talks about running Dgraph in various deployment modes, in a distributed fashion and involves
running multiple instances of Dgraph, over multiple servers in a cluster.
    </p>
    <img class="hero-deco" src="/images/hero-deco.png" />
  </div>
  <div class="item">
    <div class="icon"><i class="lni lni-checkbox" aria-hidden="true"></i></div>
    <a  href="{{< relref "production-checklist.md">}}">
      <h2>Production Checklist</h2>
      <p>
        Important setup recommendations for a production-ready Dgraph cluster
      </p>
    </a>
  </div>
  <div class="item">
    <div class="icon"><i class="lni lni-docker" aria-hidden="true"></i></div>
    <a href="{{< relref "kubernetes.md">}}">
      <h2>Using Kubernetes</h2>
      <p>
        Running Dgraph with Kubernetes
      </p>
    </a>
  </div>
  <div class="item">
    <div class="icon"><i class="lni lni-cogs" aria-hidden="true"></i></div>
    <a href="{{< relref "dgraph-administration.md">}}">
      <h2>Dgraph Administration</h2>
      <p>
        A reference to Dgraph's exposed administrative endpoints
      </p>
    </a>
  </div>

  <div class="item">
    <div class="icon"><i class="lni lni-shield" aria-hidden="true"></i></div>
    <a href="{{< relref "tls-configuration.md">}}">
      <h2>TLS Configuration</h2>
      <p>
        Setting up secure TLS connections between clients and servers 
      </p>
    </a>
  </div>
  <div class="item">
    <div class="icon"><i class="lni lni-upload" aria-hidden="true"></i></div>
    <a href="{{< relref "fast-data-loading.md">}}">
      <h2>Fast Data Loading</h2>
      <p>
        Dgraph tools for fast data loading
      </p>
    </a>
  </div>
  <div class="item">
    <div class="icon"><i class="lni lni-keyword-research" aria-hidden="true"></i></div>
    <a href="{{< relref "monitoring.md">}}">
      <h2>Monitoring</h2>
      <p>
        Dgraph's exposed metrics and monitoring endpoints
      </p>
    </a>
  </div>

</div>

<style>
  ul.contents {
    display: none;
  }
</style>
