package main

import (
	"fmt"

	"github.com/petfinder/internal"
)

func main() {
	a, b := 2, 2
	result := internal.Add(a, b)
	fmt.Printf("%d + %d = %d\n", a, b, result)
}
