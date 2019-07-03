package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
	sysconf "github.com/tklauser/go-sysconf"
)

var logger = log.New(os.Stderr, "", 0)

func getPID(pfile string) (int64, error) {
	if len(pfile) == 0 {
		logger.Fatalf("The procfile must be set")
	}
	pidData, err := ioutil.ReadFile(pfile)
	if err != nil {
		logger.Fatalf("Error while reading the pid file: %v", err)
	}

	return strconv.ParseInt(string(pidData), 0, 64)
}

// format of the /proc/<pid>/stat file can be found at
// http://man7.org/linux/man-pages/man5/proc.5.html
// Here we only define the first section of field list up to the cpu time related fields
// for easy scanning. For now, we only need the CPU fields.
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
	pfile := pflag.StringP("procfile", "p", "", "The file storing process id")
	pflag.Parse()

	pid, err := getPID(*pfile)
	if err != nil {
		glog.Fatalf("error while getting pid: %v", err)
	}

	ticker := time.NewTicker(3 * time.Second)
	go func() {
		for range ticker.C {
			refreshStat(pid)
		}
	}()
	time.Sleep(300 * time.Second)
	ticker.Stop()
	fmt.Println("Ticker stopped")
}

var hertz int64

const (
	uptime_file = "/proc/uptime"
	stat_file   = "/proc/stat"
)

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

// fileToBuf opens filename only if necessary and seeks to 0 so that
// successive calls to the functions are more efficient.
// It also reads the current content of the file into the buf
func fileToBuf(filename string, file *os.File, buf []byte) (*os.File, error) {
	if file == nil {
		var err error
		file, err = os.Open(filename)
		if err != nil {
			return nil, err
		}
	}

	_, err := file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}
	_, err = file.Read(buf)
	if err != nil {
		return nil, err
	}
	return file, nil
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
		fmt.Printf("got pcpu %v\n", float64(tics)*frame_tscale)
	}
	oldproc = *proc
}
