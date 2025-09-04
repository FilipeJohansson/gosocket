package gosocket

import (
	"fmt"
	"time"
)

func generateClientID() string {
	return fmt.Sprintf("client_%d", time.Now().UnixNano())
}
