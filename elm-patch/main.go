package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var opt_update bool
var opt_backup bool

func main() {
	flag.BoolVar(&opt_update, "update", false, "update the source files")
	flag.BoolVar(&opt_backup, "backup", false, "create backup of original source")
	flag.Parse()

	var matched bool
	var err error

	for _, fpath := range flag.Args() {
		matched = false

		for pat, patcher := range Patches {
			matched, err = regexp.MatchString(pat, filepath.ToSlash(fpath))
			if err != nil {
				fmt.Printf("unable to compile regexp: %s: %s", pat, err)
				os.Exit(1)
			}
			if matched {
				err := patchSource(fpath, patcher, opt_update, opt_backup)
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				break
			}
		}

		if !matched {
			fmt.Printf("no patch defined for fpath: %s\n", fpath)
			os.Exit(1)
		}
	}
}
