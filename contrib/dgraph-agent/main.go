package main

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
	sysconf "github.com/tklauser/go-sysconf"
)

// getPID reads the process id from the given pid file.
func getPID(pfile string) (int64, error) {
	if len(pfile) == 0 {
		glog.Fatalf("The procfile must be set")
	}
	pidData, err := ioutil.ReadFile(pfile)
	if err != nil {
		glog.Fatalf("Error while reading the pid file: %v", err)
	}

	return strconv.ParseInt(string(pidData), 0, 64)
}

// Format of the /proc/<pid>/stat file can be found at
// http://man7.org/linux/man-pages/man5/proc.5.html
// Here we only define the first section of the field list up to the cpu time related fields
// because for now we only need the CPU fields.
type proc struct {
	id      int
	name    string
	state   string
	ppid    int
	pgrp    int
	session int
	tty     int
	tpgid   int
	flags   uint
	minFlt  uint
	cminFlt uint
	majFlt  uint
	cmajFlt uint
	utime   int
	stime   int
	cutime  int
	cstime  int
}

func main() {
	pfile := pflag.StringP("procfile", "p", "",
		"The file storing id of the process to be monitored")
	pflag.Parse()

	pid, err := getPID(*pfile)
	if err != nil {
		glog.Fatalf("Error while getting pid: %v", err)
	}

	ticker := time.NewTicker(3 * time.Second)
	for range ticker.C {
		refreshStat(pid)
	}
}

var hertz int64
var smpNumCpus int64

func init() {
	var err error
	smpNumCpus, err = sysconf.Sysconf(sysconf.SC_NPROCESSORS_ONLN)
	if err != nil {
		glog.Fatalf("Unable to get the number of cpus")
	}
	hertz, err = sysconf.Sysconf(sysconf.SC_CLK_TCK)
	if err != nil {
		glog.Fatalf("Unable to get hertz")
	}
}

var oldTv syscall.Timeval
var oldProc proc

func refreshStat(pid int64) {
	proc, err := readProc(pid)
	if err != nil {
		glog.Fatalf("Error while reading proc data: %v", err)
	}
	tv := syscall.Timeval{}
	err = syscall.Gettimeofday(&tv)
	if err != nil {
		glog.Fatalf("Unable to read time: %v", err)
	}
	// et represents the elapsed time in seconds
	et := (float64)(tv.Sec-oldTv.Sec) +
		(float64(tv.Usec-oldTv.Usec))/1000000.0
	oldTv = tv

	// frameTsacel reprents the percent of cpu for each cpu tick
	frameTscale := 100.0 / (float64(hertz) * float64(et))
	if oldProc.id != 0 {
		tics := proc.utime + proc.stime - (oldProc.utime + oldProc.stime)
		// TODO: instead of printing this on the command line, expose this
		// as a promethus metric
		fmt.Printf("got pcpu %v\n", float64(tics)*frameTscale)
	}
	oldProc = *proc
}
