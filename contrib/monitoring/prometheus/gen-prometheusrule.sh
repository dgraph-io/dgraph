#!/usr/bin/env bash
# Regenerates dgraph-prometheusrule.yaml from dgraph-rules.yaml. Run it after editing the rules:
#   ./gen-prometheusrule.sh > dgraph-prometheusrule.yaml
set -euo pipefail
cd "$(dirname "$0")"
cat <<'HEADER'
# Generated from dgraph-rules.yaml by gen-prometheusrule.sh. Do not edit by hand.
#
# kube-prometheus-stack only loads PrometheusRules whose labels match its ruleSelector, which by default is
# `release: <helm release name>`. Change the label below to match your installation.
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: dgraph
  labels:
    release: kube-prometheus-stack
spec:
HEADER
grep -v '^#' dgraph-rules.yaml | sed 's/^/  /'
