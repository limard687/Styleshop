package httpapi

import "testing"

func TestBatchCartIsAtomic(t *testing.T) {
	f := newFixture(t)
	token := f.login("client")
	items := list(obj(f.request("GET", "/product", "", nil, 200))["items"])
	a, b := obj(items[0])["id"].(string), obj(items[1])["id"].(string)
	f.request("POST", "/cart/batch", token, map[string]any{"items": []map[string]any{{"product_id": a, "count": 2}, {"product_id": b, "count": 99}}}, 400)
	var n int
	f.db.QueryRow("SELECT count(*) FROM carts").Scan(&n)
	if n != 0 {
		t.Fatal("partial addition", n)
	}
	f.request("POST", "/cart/batch", token, map[string]any{"items": []map[string]any{{"product_id": a, "count": 2}, {"product_id": b, "count": 3}}}, 201)
	var sum int
	f.db.QueryRow("SELECT count(*),SUM(count) FROM carts").Scan(&n, &sum)
	if n != 2 || sum != 5 {
		t.Fatal(n, sum)
	}
	f.request("POST", "/cart/batch", f.login("admin"), map[string]any{}, 403)
}
