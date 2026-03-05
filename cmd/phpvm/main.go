package main

import "fmt"

var version = "0.1.0-dev"

func main() {
	fmt.Println("phpvm", version)
	fmt.Println("Use: phpvm <install|use|list|current|available|remove|version>")
}
