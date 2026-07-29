// hostid-check is a dev tool. Prints the host-id of the current machine so
// one can verify it is stable and identical across runs.
// Not part of the main build.
package main

import (
	"fmt"

	"github.com/velesbsdllc/agent-vbai/internal/hostid"
)

func main() {
	id, err := hostid.Get()
	fmt.Printf("hostid = %s\nerr    = %v\nlen    = %d\n", id, err, len(id))
}
