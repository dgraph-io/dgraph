// +build !noasm
// +build amd64

package codec

import (
	"github.com/golang/glog"
)

func init() {
	glog.Infof("[Decoder]: Using assembly version of decoder")
}
