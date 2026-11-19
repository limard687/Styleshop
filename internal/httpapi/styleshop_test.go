package httpapi

import (
	"clothing-store/internal/config"
	"clothing-store/internal/store"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func TestStyleShopOriginalCatalogAndCheckout(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for i := 0; i < 2; i++ {
		if err = store.SeedStyleShop(db, "StyleDemo2026!"); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{UploadDir: filepath.Join(dir, "uploads"), SessionSecret: strings.Repeat("s", 32), SeedDemo: true}
	app, err := New(db, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{app, db, t}
	catalog := obj(f.request("GET", "/product?limit=100", "", nil, 200))
	if catalog["total"] != float64(6) {
		t.Fatal(catalog)
	}
	const jeans = "e87072c3-6590-45e9-9586-9f9e852227c9"
	p := obj(f.request("GET", "/product/"+jeans, "", nil, 200))
	if p["price"] != float64(225000) || p["stock"] != float64(7) || len(list(p["photos"])) != 3 {
		t.Fatal(p)
	}
	client, manager := f.login("client"), f.login("manager")
	f.request("POST", "/cart", manager, map[string]any{"product_id": jeans, "count": 1}, 403)
	f.request("POST", "/cart", client, map[string]any{"product_id": jeans, "count": 2}, 201)
	order := obj(f.request("POST", "/order", client, map[string]any{}, 201))
	id := order["id"].(string)
	p = obj(f.request("GET", "/product/"+jeans, "", nil, 200))
	if p["stock"] != float64(5) {
		t.Fatal(p)
	}
	f.request("PUT", "/order/"+id, manager, map[string]string{"status": "Отменён"}, 400)
	f.request("PUT", "/order/"+id, manager, map[string]string{"status": "Отменён", "cancellation_reason": "Проверка возврата"}, 200)
	p = obj(f.request("GET", "/product/"+jeans, "", nil, 200))
	if p["stock"] != float64(7) {
		t.Fatal("cancel did not return original stock", p)
	}
	// Repeated startup must not reset stock, restore removed photos, or duplicate data.
	if _, err = db.Exec("UPDATE products SET stock=3 WHERE id=?", jeans); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("DELETE FROM photos WHERE product_id=?", jeans); err != nil {
		t.Fatal(err)
	}
	if err = store.SeedStyleShop(db, "StyleDemo2026!"); err != nil {
		t.Fatal(err)
	}
	if _, err = New(db, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	p = obj(f.request("GET", "/product/"+jeans, "", nil, 200))
	if p["stock"] != float64(3) || len(list(p["photos"])) != 0 {
		t.Fatal(p)
	}
}
