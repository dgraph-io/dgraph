// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"text/template"
	"time"

	"github.com/ChainSafe/gossamer/lib/utils"
	"github.com/docker/docker/pkg/reexec"
	"github.com/stretchr/testify/require"
)

type TestExecCommand struct {
	*testing.T
	Func    template.FuncMap
	Data    interface{}
	Cleanup func()
	cmd     *exec.Cmd
	stdout  *bufio.Reader
	stdin   io.WriteCloser
	stderr  *testlog
	Err     error
}

type testgossamer struct {
	*TestExecCommand
	Datadir   string
	Etherbase string
}

type testlog struct {
	t   *testing.T
	mu  sync.Mutex
	buf bytes.Buffer
}

func init() {
	reexec.Register("gossamer-test", func() {
		if err := app.Run(os.Args); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	})
}

func (tl *testlog) Write(b []byte) (n int, err error) {
	lines := bytes.Split(b, []byte("\n"))
	for _, line := range lines {
		if len(line) > 0 {
			tl.t.Logf("stderr: %s", line)
		}
	}
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.buf.Write(b)
	return len(b), err
}

func (tt *TestExecCommand) Run(name string, args ...string) {
	tt.stderr = &testlog{t: tt.T}
	tt.cmd = &exec.Cmd{
		Path:   reexec.Self(),
		Args:   append([]string{name}, args...),
		Stderr: tt.stderr,
	}
	stdout, err := tt.cmd.StdoutPipe()
	require.Nil(tt, err)
	tt.stdout = bufio.NewReader(stdout)
	if tt.stdin, err = tt.cmd.StdinPipe(); err != nil {
		require.Nil(tt, err)
	}
	if err := tt.cmd.Start(); err != nil {
		require.Nil(tt, err)
	}
}

func (tt *TestExecCommand) ExpectExit() {
	var output []byte
	tt.withKillTimeout(func() {
		output, _ = ioutil.ReadAll(tt.stdout)
	})
	tt.WaitExit()
	if tt.Cleanup != nil {
		tt.Cleanup()
	}
	if len(output) > 0 {
		tt.Errorf("stdout unmatched:\n%s", output)
	}
}

func (tt *TestExecCommand) GetOutput() (stdout []byte, stderr []byte) {
	tt.withSigTimeout(func() {
		stdout, _ = ioutil.ReadAll(tt.stdout)
		stderr = tt.stderr.buf.Bytes()
	})
	tt.WaitExit()
	if tt.Cleanup != nil {
		tt.Cleanup()
	}

	return stdout, stderr
}

func (tt *TestExecCommand) WaitExit() {
	tt.Err = tt.cmd.Wait()
}

func (tt *TestExecCommand) withKillTimeout(fn func()) {
	timeout := time.AfterFunc(5*time.Second, func() {
		tt.Log("process timeout, killing")
		tt.Kill()
	})
	defer timeout.Stop()
	fn()
}

func (tt *TestExecCommand) withSigTimeout(fn func()) {
	timeout := time.AfterFunc(5*time.Second, func() {
		tt.Log("process timeout, will signal")
		tt.Signal()
	})
	defer timeout.Stop()
	fn()
}

func (tt *TestExecCommand) Kill() {
	_ = tt.cmd.Process.Kill()
	if tt.Cleanup != nil {
		tt.Cleanup()
	}
}

func (tt *TestExecCommand) Signal() {
	err := tt.cmd.Process.Signal(syscall.SIGINT)
	require.Nil(tt.T, err)
	if tt.Cleanup != nil {
		tt.Cleanup()
	}
}

func (tt *TestExecCommand) Expect(tplsource string) {
	tpl := template.Must(template.New("").Funcs(tt.Func).Parse(tplsource))
	wantbuf := new(bytes.Buffer)
	require.Nil(tt, tpl.Execute(wantbuf, tt.Data))

	want := bytes.TrimPrefix(wantbuf.Bytes(), []byte("\n"))
	tt.matchExactOutput(want)

	tt.Logf("stdout matched:\n%s", want)
}

func (tt *TestExecCommand) matchExactOutput(want []byte) {
	buf := make([]byte, len(want))
	n := 0
	tt.withKillTimeout(func() { n, _ = io.ReadFull(tt.stdout, buf) })
	buf = buf[:n]
	if n < len(want) || !bytes.Equal(buf, want) {
		buf = append(buf, make([]byte, tt.stdout.Buffered())...)
		_, _ = tt.stdout.Read(buf[n:])
		require.Equal(tt, want, buf)
	}
}

func (tt *TestExecCommand) StderrText() string {
	tt.stderr.mu.Lock()
	defer tt.stderr.mu.Unlock()
	return tt.stderr.buf.String()
}

func newTestCommand(t *testing.T, data interface{}) *TestExecCommand {
	return &TestExecCommand{T: t, Data: data}
}

func runTestGossamer(t *testing.T, args ...string) *testgossamer {
	tt := &testgossamer{}
	tt.TestExecCommand = newTestCommand(t, tt)
	tt.Run("gossamer-test", args...)
	return tt
}

func TestMain(m *testing.M) {
	if reexec.Init() {
		return
	}
	os.Exit(m.Run())
}

func TestInvalidCommand(t *testing.T) {
	gossamer := runTestGossamer(t, "potato")

	gossamer.ExpectExit()

	expectedMessages := []string{
		"failed to read command argument: \"potato\"",
	}

	for _, m := range expectedMessages {
		require.Contains(t, gossamer.StderrText(), m)
	}
}

func TestGossamerCommand(t *testing.T) {
	t.Skip() // TODO: not sure how relevant this is anymore, it also slows down the tests a lot

	basePort := 7000
	genesisPath := utils.GetGssmrGenesisRawPath()

	tempDir, err := ioutil.TempDir("", "gossamer-maintest-")
	require.Nil(t, err)

	gossamer := runTestGossamer(t,
		"init",
		"--basepath", tempDir,
		"--genesis", genesisPath,
		"--force",
	)

	stdout, stderr := gossamer.GetOutput()
	t.Log("init gossamer output, ", "stdout", string(stdout), "stderr", string(stderr))

	expectedMessages := []string{
		"node initialized",
	}

	for _, m := range expectedMessages {
		require.Contains(t, string(stdout), m)
	}

	for i := 0; i < 10; i++ {
		t.Log("Going to gossamer cmd", "iteration", i)

		// start
		gossamer = runTestGossamer(t,
			"--port", strconv.Itoa(basePort),
			"--key", "alice",
			"--basepath", tempDir,
			"--roles", "4",
		)

		time.Sleep(10 * time.Second)

		stdout, stderr = gossamer.GetOutput()
		log.Println("Run gossamer output, ", "stdout", string(stdout), "stderr", string(stderr))

		expectedMessages = []string{
			"SIGABRT: abort",
		}

		for _, m := range expectedMessages {
			require.NotContains(t, string(stderr), m)
		}
	}

}

// TODO: TestExportCommand test "gossamer export" does not error

// TODO: TestInitCommand test "gossamer init" does not error

// TODO: TestAccountCommand test "gossamer account" does not error
