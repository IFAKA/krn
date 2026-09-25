package catalog

type Product struct {
	SKU   string
	Name  string
	Price int
}

func Default() []Product {
	return []Product{
		{SKU: "food-001", Name: "Salmon bites", Price: 1299},
		{SKU: "toy-002", Name: "Rope toy", Price: 899},
	}
}

func Total(products []Product) int {
	total := 0
	for _, product := range products {
		total += product.Price
	}
	return total
}
