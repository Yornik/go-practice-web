package fixture

type Stock struct {
	Qty      int
	Reserved int
}

func Reserve(stock map[string]Stock, sku string, n int) {
	stock[sku].Qty -= n
	stock[sku].Reserved += n
}
