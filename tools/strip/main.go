package main

import (
	"flag"
	"fmt"
	"github.com/apstndb/gsqlutils"
	"github.com/k0kubun/pp"
)

func main() {
	flag.Parse()
	s := flag.Args()[0]
	fmt.Println("input:", s)
	status, err := gsqlutils.SeparateInputPreserveCommentsWithStatus("", s)
	fmt.Println("output:", pp.Sprint(status))
	fmt.Println("err:", err)
}
