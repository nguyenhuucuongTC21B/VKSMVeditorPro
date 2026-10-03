// Command selftest — wrapper mỏng gọi internal/selftest (dùng cho Linux/dev).
package main

import (
	"os"

	"vkseditorpro/internal/selftest"
)

func main() {
	os.Exit(selftest.Run(os.Stdout))
}
