// Command terraform-provider-omada is the omada Terraform provider's plugin
// binary.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/filipegalo/terraform-provider-omada/internal/provider"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers")
	flag.Parse()

	// The registry address this provider is published under. OpenTofu resolves
	// registry.terraform.io addresses too, so one address serves both.
	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/filipegalo/omada",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
