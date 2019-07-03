package main

import (
	"fmt"
	"log"
	"os"
)

func old_Hertz_hack() {
	var user_j, nice_j, sys_j, other_j uint64 /* jiffies (clock ticks) */
	var up_1, up_2, seconds float64
	buf := make([]byte, 1024)
	var uptimeFile, statFile *os.File
	var err error
	for {
		uptimeFile, err = fileToBuf(uptime_file, uptimeFile, buf)
		if err != nil {
			log.Fatalf("unable to read the %s", uptime_file)
		}
		fmt.Sscanf(string(buf), "%f", &up_1)

		statFile, err = fileToBuf(stat_file, statFile, buf)
		if err != nil {
			log.Fatalf("unable to read the %s", stat_file)
		}
		fmt.Sscanf(string(buf), "cpu %d %d %d %d", &user_j, &nice_j, &sys_j, &other_j)

		uptimeFile, err = fileToBuf(uptime_file, uptimeFile, buf)
		fmt.Sscanf(string(buf), "%f", &up_2)

		if (uint64)((up_2-up_1)*1000.0/up_1) == 0 {
			/* want under 0.1% error */
			break
		}
	}

	jiffies := user_j + nice_j + sys_j + other_j
	seconds = (up_1 + up_2) / 2
	h := uint32(float64(jiffies) / float64(seconds) / float64(smp_num_cpus))
	/* actual values used by 2.4 kernels: 32 64 100 128 1000 1024 1200 */
	switch {
	case h >= 9 && h <= 11:
		hertz = 10 /* S/390 (sometimes) */
	case h >= 18 && h <= 22:
		hertz = 20 /* user-mode Linux */
	case h >= 30 && h <= 34:
		hertz = 32 /* ia64 emulator */
	case h >= 48 && h <= 52:
		hertz = 50
	case h >= 58 && h <= 61:
		hertz = 60
	case h >= 62 && h <= 65:
		hertz = 64 /* StrongARM /Shark */
	case h >= 95 && h <= 105:
		hertz = 100 /* normal Linux */
	case h >= 124 && h <= 132:
		hertz = 128 /* MIPS, ARM */
	case h >= 195 && h <= 204:
		hertz = 200 /* normal << 1 */
	case h >= 247 && h <= 252:
		hertz = 250
	case h >= 253 && h <= 260:
		hertz = 256
	case h >= 393 && h <= 408:
		hertz = 400 /* normal << 2 */
	case h >= 790 && h <= 808:
		hertz = 800 /* normal << 3 */
	case h >= 990 && h <= 1010:
		hertz = 1000 /* ARM */
	case h >= 1015 && h <= 1035:
		hertz = 1024 /* Alpha, ia64 */
	case h >= 1180 && h <= 1220:
		hertz = 1200 /* Alpha */
	default:
		log.Fatalf("Unknown HZ value! (%d) Assume %Ld.\n", h, hertz)
	}
}
