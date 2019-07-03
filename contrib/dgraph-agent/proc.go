package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
)

func readProc(pid int64) (*proc, error) {
	pstatFile := fmt.Sprintf("/proc/%d/stat", pid)
	file, err := os.Open(pstatFile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		p := proc{}
		fmt.Sscanf(line,
			"%d %s %s "+
				"%d %d %d %d %d "+
				"%d %d %d %d %d "+
				"%d %d %d %d ", /* utime stime cutime cstime */
			&p.id, &p.name, &p.state,
			&p.ppid, &p.pgrp, &p.session, &p.tty, &p.tpgid,
			&p.flags, &p.min_flt, &p.cmin_flt, &p.maj_flt, &p.cmaj_flt,
			&p.utime, &p.stime, &p.cutime, &p.cstime,
		)
		return &p, nil
	}
	return nil, fmt.Errorf("unable to read line from the file %s", pstatFile)
}
