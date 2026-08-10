package cmd

import _ "embed"

//go:embed embedded/LICENSE
var licenseText string

func Run() { _ = licenseText }
