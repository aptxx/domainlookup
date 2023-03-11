// Package main gets domain names from a file
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

// flags
var (
	fFile string
)

func init() {
	flag.StringVar(&fFile, "f", "", "File contains domain, one domain per line")
}

var regexDomain = regexp.MustCompile(`^([a-z0-9]+(-[a-z0-9]+)*)+\.[a-z]{2,}$`)

func find(line string) (domain string) {
	for _, word := range strings.Split(line, ",") {
		if regexDomain.MatchString(word) {
			return word
		}
	}
	return
}

func main() {
	flag.Parse()

	if fFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Open the file
	file, err := os.Open(fFile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Create a new scanner and read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if domain := find(scanner.Text()); domain != "" {
			fmt.Println(domain)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}
