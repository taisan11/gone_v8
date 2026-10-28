package main

import (
	"fmt"
	"log"

	v8 "github.com/taisan11/gone_v8"
)

func main() {
	engine, err := v8.New()
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()

	for _, script := range []string{
		"1 + 2 * 3",
		"globalThis.count = (globalThis.count || 0) + 1; count",
		"globalThis.count = (globalThis.count || 0) + 1; count",
		"(() => { throw new Error('sample exception') })()",
	} {
		value, err := engine.Eval(script)
		fmt.Printf("%s => result=%q err=%v\n", script, value, err)
	}
}
