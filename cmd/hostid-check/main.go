// hostid-check — dev-инструмент. Печатает host-id для текущей машины,
// чтобы можно было проверить что стабильный и одинаковый между запусками.
// Не входит в основной билд.
package main

import (
	"fmt"

	"github.com/velesbsdllc/agent-vbai/internal/hostid"
)

func main() {
	id, err := hostid.Get()
	fmt.Printf("hostid = %s\nerr    = %v\nlen    = %d\n", id, err, len(id))
}
