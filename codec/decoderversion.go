// +build !noasm
// +build amd64

package codec

import "fmt"

func init() {
	fmt.Println("[Decoder]: Using assembly version of decoder")
}
