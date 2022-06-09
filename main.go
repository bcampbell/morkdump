package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
)

func main() {

	flag.Parse()

	for _, inFilename := range flag.Args() {
		tabs, err := slurp(inFilename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
			os.Exit(1)
		}
		err = output(tabs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
			os.Exit(1)
		}

	}
}

func slurp(filename string) (map[string]Table, error) {
	raw, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	l := newLexer(raw)
	p := NewParser(filename, l)
	return p.Parse()
}

func dumpToks(filename string) error {
	raw, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	l := newLexer(raw)
	for {
		item := l.nextItem()
		fmt.Printf("%s\n", item)
		if item.typ == tEOF {
			return nil
		}
		if item.typ == tERROR {
			return fmt.Errorf("Poop.\n")
		}
	}
}

func output(tabs map[string]Table) error {

	for toid, tab := range tabs {
		fmt.Printf("----- %s -----\n", toid)
		for roid, row := range tab.rows {
			fmt.Printf("  row %s:\n", roid)
			for name, value := range row {
				fmt.Printf("    %s: '%s'\n", name, value)
			}
		}
	}

	return nil
}
