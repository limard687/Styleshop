package httpapi

import "testing"

func TestInventoryReservationsAndReturns(t *testing.T) {
	f := newFixture(t)
	admin, client, manager := f.login("admin"), f.login("client"), f.login("manager")
	body := map[string]any{"name": "Складская модель", "description": "Описание", "price": 10000, "attributes": map[string][]string{"type": {"Худи"}, "gender": {"Унисекс"}, "size": {"S", "M"}, "color": {"Чёрный"}, "brand": {"StyleShop"}, "season": {"Лето"}}, "stock_by_variant": map[string]map[string]int{"S": {"Чёрный": 2}, "M": {"Чёрный": 5}}}
	created := obj(f.request("POST", "/product", admin, body, 201))
	ids := list(created["variant_ids"])
	first, second := ids[0].(string), ids[1].(string)
	stock := func(id string) float64 { return obj(f.request("GET", "/product/"+id, "", nil, 200))["stock"].(float64) }
	if stock(first) != 2 || stock(second) != 5 {
		t.Fatal("variant stock mismatch")
	}
	f.request("PATCH", "/product/"+first+"/stock", client, map[string]int{"stock": 10, "expected_stock": 2}, 403)
	f.request("POST", "/cart", client, map[string]any{"product_id": first, "count": 3}, 400)
	f.request("POST", "/cart", client, map[string]any{"product_id": first, "count": 2}, 201)
	f.request("POST", "/cart", client, map[string]any{"product_id": second, "count": 1}, 201)
	f.request("PATCH", "/product/"+second+"/stock", manager, map[string]int{"stock": 0, "expected_stock": 5}, 403)
	f.request("PATCH", "/product/"+second+"/stock", admin, map[string]int{"stock": 0, "expected_stock": 5}, 200)
	f.request("POST", "/order", client, map[string]any{}, 400)
	if stock(first) != 2 {
		t.Fatal("failed checkout partially deducted stock")
	}
	f.request("PATCH", "/product/"+second+"/stock", admin, map[string]int{"stock": 5, "expected_stock": 5}, 409)
	f.request("PATCH", "/product/"+second+"/stock", admin, map[string]int{"stock": 5, "expected_stock": 0}, 200)
	order := obj(f.request("POST", "/order", client, map[string]any{}, 201))
	if stock(first) != 0 || stock(second) != 4 {
		t.Fatal("checkout stock mismatch")
	}
	f.request("POST", "/cart", client, map[string]any{"product_id": first, "count": 1}, 400)
	path := "/order/" + order["id"].(string)
	f.request("PUT", path, admin, map[string]string{"status": "Отменён", "cancellation_reason": "Товар повреждён"}, 200)
	if stock(first) != 2 || stock(second) != 5 {
		t.Fatal("cancel did not return stock")
	}
	f.request("PUT", path, admin, map[string]string{"status": "Отменён"}, 200)
	if stock(first) != 2 || stock(second) != 5 {
		t.Fatal("repeat cancellation returned stock twice")
	}
}
