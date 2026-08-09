// Command openapi prints the Boreas OpenAPI 3 document as YAML.
package main

import (
	"fmt"
	"os"

	httptransport "github.com/zenkiet/boreas/internal/transport/http"
)

func main() {
	spec, err := httptransport.MarshalOpenAPIYAML()
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate openapi:", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(spec); err != nil {
		fmt.Fprintln(os.Stderr, "write openapi:", err)
		os.Exit(1)
	}
}
