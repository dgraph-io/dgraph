package x

import "sync/atomic"

var (
	healthCheck uint32
	memoryCheck uint32
)

func UpdateMemoryStatus(ok bool) {
	setStatus(&memoryCheck, ok)
}

func UpdateHealthStatus(ok bool) {
	setStatus(&healthCheck, ok)
}

func setStatus(v *uint32, ok bool) {
	if ok {
		atomic.StoreUint32(v, 1)
	} else {
		atomic.StoreUint32(v, 0)
	}
}

// HealthCheck returns whether the server is ready to accept requests or not
// Load balancer would add the node to the endpoint once health check starts
// returning true
func HealthCheck() bool {
	return atomic.LoadUint32(&memoryCheck) != 0 && atomic.LoadUint32(&healthCheck) != 0
}

func init() {
	memoryCheck = 1
}
