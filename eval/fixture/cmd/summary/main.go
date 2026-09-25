package main

import (
	"fmt"

	"example.com/fixture/catalog"
)

func main() {
	for _, product := range catalog.Default() {
		fmt.Printf("%s=%d\n", product.SKU, product.Price)
	}
}
