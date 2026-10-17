package fixture

type Stock struct{ Qty int }

func Item(stock map[string]Stock, sku string) *Stock {
	return &stock[sku]
}
