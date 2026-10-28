package main

import (
	"fmt"
	"log"

	gone_v8 "github.com/taisan11/gone_v8/lowlevel"
)

func main() {
	// No worker: the calling goroutine stays on one OS thread for this callback.
	err := gone_v8.WithIsolate(func(isolate *gone_v8.Isolate) error {
		err := isolate.WithScope(func(scope *gone_v8.Scope) error {
			_, err := scope.Eval("globalThis.counter = 0")
			return err
		})
		if err != nil {
			return err
		}
		script, err := isolate.Compile("++counter")
		if err != nil {
			return err
		}
		defer script.Close()

		// The script survives scopes, but each returned Value is scope-local.
		for n := 0; n < 3; n++ {
			if err := isolate.WithScope(func(scope *gone_v8.Scope) error {
				value, err := script.Run(scope)
				if err != nil {
					return err
				}
				number, err := value.Float64() // No ToString or UTF-8 copy.
				if err != nil {
					return err
				}
				fmt.Printf("compiled run => %g\n", number)
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
