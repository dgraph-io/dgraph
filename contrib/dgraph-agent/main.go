package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
	sysconf "github.com/tklauser/go-sysconf"
)

// getPID reads the process id from the given pid file
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

// format of the /proc/<pid>/stat file can be found at
// http://man7.org/linux/man-pages/man5/proc.5.html
// Here we only define the first section of field list up to the cpu time related fields
// because for now we only need the CPU fields.
type proc struct {
	id       int
	name     string
	state    string
	ppid     int
	pgrp     int
	session  int
	tty      int
	tpgid    int
	flags    uint
	min_flt  uint
	cmin_flt uint
	maj_flt  uint
	cmaj_flt uint
	utime    int
	stime    int
	cutime   int
	cstime   int
}

func main() {
	pfile := pflag.StringP("procfile", "p", "",
		"The file storing id of the process to be monitored")
	pflag.Parse()

	pid, err := getPID(*pfile)
	if err != nil {
		glog.Fatalf("error while getting pid: %v", err)
	}

	ticker := time.NewTicker(3 * time.Second)
	for range ticker.C {
		refreshStat(pid)
	}
}

var hertz int64
var smp_num_cpus int64

func init() {
	var err error
	smp_num_cpus, err = sysconf.Sysconf(sysconf.SC_NPROCESSORS_ONLN)
	if err != nil {
		log.Fatalf("unable to get the number of cpus")
	}
	hertz, err = sysconf.Sysconf(sysconf.SC_CLK_TCK)
	if err != nil {
		log.Fatalf("unable to get hertz")
	}
}

var oldtv syscall.Timeval
var oldproc proc

func refreshStat(pid int64) {
	proc, err := readProc(pid)
	if err != nil {
		glog.Fatalf("error while reading proc data: %v", err)
	}
	tv := syscall.Timeval{}
	syscall.Gettimeofday(&tv)
	// et represents the elapsed time in seconds
	et := (float64)(tv.Sec-oldtv.Sec) +
		(float64(tv.Usec-oldtv.Usec))/1000000.0
	oldtv = tv

	// frame_tsacel reprents the percent of cpu for each cpu tick
	frame_tscale := 100.0 / (float64(hertz) * float64(et))
	if oldproc.id != 0 {
		tics := proc.utime + proc.stime - (oldproc.utime + oldproc.stime)
		// TODO: instead of printing this on the command line, expose this
		// as a promethus metric
		fmt.Printf("got pcpu %v\n", float64(tics)*frame_tscale)
	}
	oldproc = *proc
}
