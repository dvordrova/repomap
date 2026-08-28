package model

type Address struct {
	Street     string `json:"street"`
	City       string `json:"city"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

type Item struct {
	SKU         string `json:"sku"`
	UnitCents   int64  `json:"unit_cents"`
	Quantity    int    `json:"quantity"`
	WeightGrams int    `json:"weight_grams"`
}

type Order struct {
	ID       string  `json:"id"`
	Email    string  `json:"email"`
	Currency string  `json:"currency"`
	ShipTo   Address `json:"ship_to"`
	Items    []Item  `json:"items"`
}

func (o Order) SubtotalCents() int64 {
	var total int64
	for _, item := range o.Items {
		total += item.UnitCents * int64(item.Quantity)
	}
	return total
}

func (o Order) WeightGrams() int {
	var total int
	for _, item := range o.Items {
		total += item.WeightGrams * item.Quantity
	}
	return total
}

type Fulfilment struct {
	OrderID      string `json:"order_id"`
	ApprovalCode string `json:"approval_code"`
	TaxCents     int64  `json:"tax_cents"`
	TrackingCode string `json:"tracking_code"`
}
