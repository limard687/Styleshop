package httpapi

import "testing"

func TestSequentialOrderNumbersAndBuyerContents(t *testing.T) {
	f := newFixture(t)
	client, admin := f.login("client"), f.login("admin")
	product := obj(list(obj(f.request("GET", "/product", "", nil, 200))["items"])[0])
	for number := 1; number <= 2; number++ {
		f.request("POST", "/order", client, map[string]any{}, 400)
		f.request("POST", "/cart", client, map[string]any{"product_id": product["id"], "count": 2}, 201)
		order := obj(f.request("POST", "/order", client, map[string]any{}, 201))
		if order["order_number"] != float64(number) {
			t.Fatal(order)
		}
		for _, token := range []string{client, admin} {
			detail := obj(f.request("GET", "/order/"+order["id"].(string), token, nil, 200))
			if detail["order_number"] != float64(number) {
				t.Fatal(detail)
			}
			items := list(detail["items"])
			if len(items) != 1 {
				t.Fatal(items)
			}
			item := obj(items[0])
			if item["name"] != product["name"] || item["size"] != product["size"] || item["color"] != product["color"] || item["price"] != product["price"] || item["count"] != float64(2) {
				t.Fatal(item)
			}
		}
	}
	for _, token := range []string{client, admin} {
		orders := list(obj(f.request("GET", "/order", token, nil, 200))["items"])
		if len(orders) != 2 || obj(orders[0])["order_number"] != float64(2) || obj(orders[1])["order_number"] != float64(1) {
			t.Fatal(orders)
		}
	}
}
