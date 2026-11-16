// Command dlg downloads and decrypts the lastgood.xml configuration backup of a
// Unionman/CMCC UNG953H1-S optical modem.
package main

import (
	"os"

	"github.com/12ain/decrypt-lastgood/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
