/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var (
	runtimeOnce sync.Once
	runtimeName string
)

// ContainerRuntime returns the container runtime CLI to use: "docker" or "podman".
// Detection order:
//  1. CONTAINER_RUNTIME env var override
//  2. "docker" binary that reports itself as Podman → "podman"
//  3. "docker" binary (real Docker) → "docker"
//  4. "podman" binary (no docker) → "podman"
//  5. fallback → "docker"
//
// When Podman is the runtime, DOCKER_HOST is automatically set to the
// Podman socket so the Docker API SDK can communicate with it.
func ContainerRuntime() string {
	runtimeOnce.Do(func() {
		runtimeName = detectRuntime()
		if runtimeName == "podman" {
			ensurePodmanSocket()
		}
	})
	return runtimeName
}

// ContainerComposeCmdPrefix returns the base command-line slice for compose
// operations. Callers append their subcommand and flags:
//
//	prefix := ContainerComposeCmdPrefix()
//	cmd := exec.Command(prefix[0], append(prefix[1:], "-p", project, "up", "-d")...)
//
// Docker:  ["docker", "compose", "--compatibility"]
// Podman:  ["podman", "compose"]
func ContainerComposeCmdPrefix() []string {
	if ContainerRuntime() == "podman" {
		return []string{"podman", "compose"}
	}
	return []string{"docker", "compose", "--compatibility"}
}

func detectRuntime() string {
	if rt := os.Getenv("CONTAINER_RUNTIME"); rt != "" {
		return rt
	}
	if _, err := exec.LookPath("docker"); err == nil {
		out, _ := exec.Command("docker", "--version").Output()
		if strings.Contains(strings.ToLower(string(out)), "podman") {
			return "podman"
		}
		return "docker"
	}
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	return "docker"
}

// EnsureCoverageOutput returns env with COVERAGE_OUTPUT guaranteed to be
// present. When the variable is already set (coverage mode active) it is left
// unchanged. When it is missing, it is added as an empty string.
//
// Podman-compose expands an *unset* ${COVERAGE_OUTPUT} as the literal two-
// character string '""', which shlex then passes to dgraph as an empty-string
// argument before the subcommand ("dgraph" + ["", "zero", ...]). Docker Compose
// silently collapses the extra whitespace; podman-compose does not. Setting the
// variable to the empty string forces podman-compose to use the actual value
// (empty → no token) instead of the quoted placeholder.
func EnsureCoverageOutput(env []string) []string {
	for _, e := range env {
		if strings.HasPrefix(e, "COVERAGE_OUTPUT=") {
			return env // already set; leave it alone
		}
	}
	return append(env, "COVERAGE_OUTPUT=")
}

// ensurePodmanSocket sets DOCKER_HOST to the Podman socket so the Docker
// SDK (github.com/docker/docker/client) can connect to Podman's API server.
// It is a no-op when DOCKER_HOST is already set or the socket does not exist.
func ensurePodmanSocket() {
	if os.Getenv("DOCKER_HOST") != "" {
		return
	}
	xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntime == "" {
		xdgRuntime = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	sock := xdgRuntime + "/podman/podman.sock"
	if _, err := os.Stat(sock); err == nil {
		os.Setenv("DOCKER_HOST", "unix://"+sock)
	}
}
