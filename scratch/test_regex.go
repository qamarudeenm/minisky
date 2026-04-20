package main

import (
	"fmt"
	"regexp"
)

func main() {
	s := "SELECT * FROM `local-dev.ok_ds.ok_tb`;"
	fmt.Printf("0: %s\n", s)
	
	// 1. Backticks to quotes
	s = regexp.MustCompile("`").ReplaceAllString(s, "")
	fmt.Printf("1: %s\n", s)
	
	// 2. project.dataset.table -> dataset.table
	projectRe := regexp.MustCompile(`([a-zA-Z0-9_-]+)\.([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]+)`)
	s = projectRe.ReplaceAllString(s, "$2.$3")
	fmt.Printf("2: %s\n", s)
	
	// 3. dataset.table -> dataset__table
	datasetRe := regexp.MustCompile(`([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]+)`)
	s = datasetRe.ReplaceAllString(s, "${1}__$2")
	fmt.Printf("3: %s\n", s)
}
