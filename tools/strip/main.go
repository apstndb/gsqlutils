package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"slices"

	"github.com/k0kubun/pp"

	"github.com/apstndb/gsqlutils"
)

func wrapFunc[T any](f func(filepath, s string) (T, error)) func(string, string) (any, error) {
	return func(filepath string, s string) (any, error) {
		return f(filepath, s)
	}
}

var funcMap = map[string]func(string, string) (any, error){
	"simple_strip_comments":                        wrapFunc(gsqlutils.SimpleStripComments),
	"simple_skip_hints":                            wrapFunc(gsqlutils.SimpleSkipHints),
	"separate_input_preserve_comments_with_status": wrapFunc(gsqlutils.SeparateInputPreserveCommentsWithStatus),
}

func main() {
	mode := flag.String("mode", "strip", "strip or preserve")
	flag.Parse()
	var s string
	if flag.NArg() > 0 {
		s = flag.Args()[0]
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalln(err)
		}
		s = string(b)
	}

	fmt.Println("input: ", s)

	var result any
	var err error

	if funcMap[*mode] != nil {
		result, err = funcMap[*mode]("", s)
	} else {
		log.Fatalln("mode should be one of:", slices.Collect(maps.Keys(funcMap)))
	}

	if s, ok := result.(string); ok {
		fmt.Println("output:", s)
	} else {
		fmt.Println("output:", pp.Sprint(result))
	}
	fmt.Println("err:", err)
}
