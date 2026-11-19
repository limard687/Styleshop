package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"clothing-store/internal/config"
	"clothing-store/internal/store"
	"clothing-store/internal/testsupport"
)

type fixture struct {
	app *App
	db  *sql.DB
	t   *testing.T
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = testsupport.Seed(db, "StyleDemo2026!"); err != nil {
		t.Fatal(err)
	}
	a, err := New(db, config.Config{UploadDir: filepath.Join(dir, "uploads"), SessionSecret: strings.Repeat("s", 32)}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{a, db, t}
}
func (f *fixture) request(method, path, token string, body any, want int) any {
	f.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			f.t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, &buf)
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.app.ServeHTTP(w, r)
	if w.Code != want {
		f.t.Fatalf("%s %s: got %d, want %d; %s", method, path, w.Code, want, w.Body.String())
	}
	var result any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		f.t.Fatalf("bad JSON: %v; %s", err, w.Body.String())
	}
	return result
}
func (f *fixture) login(role string) string {
	f.t.Helper()
	return f.request("POST", "/auth/login", "", map[string]string{"email": role + "@styleshop.local", "password": "StyleDemo2026!"}, 200).(map[string]any)["token"].(string)
}
func obj(v any) map[string]any { return v.(map[string]any) }
func list(v any) []any         { return v.([]any) }

func TestStoreJourneyAndAccessControl(t *testing.T) {
	f := newFixture(t)
	admin, manager, client := f.login("admin"), f.login("manager"), f.login("client")
	clientID := "10000000-0000-4000-8000-000000000001"
	managerID := "10000000-0000-4000-8000-000000000002"
	t.Run("public catalog filters pagination sort", func(t *testing.T) {
		data := obj(f.request("GET", "/product?limit=2&page=2", "", nil, 200))
		if data["total"] != float64(18) || len(list(data["items"])) != 2 {
			t.Fatal(data)
		}
		for key, value := range map[string]string{"type": "Худи", "gender": "Унисекс", "size": "M", "color": "Чёрный", "brand": "StyleShop", "season": "Лето"} {
			d := obj(f.request("GET", "/product?"+url.Values{key: {value}}.Encode(), "", nil, 200))
			if d["total"].(float64) == 0 {
				t.Fatalf("filter %s failed", key)
			}
		}
		filtered := obj(f.request("GET", "/product?"+url.Values{"type": {"Худи"}, "size": {"M"}, "color": {"Угольный"}, "brand": {"StyleShop"}, "gender": {"Унисекс"}, "season": {"Зима"}}.Encode(), "", nil, 200))
		if filtered["total"] != float64(1) {
			t.Fatal(filtered)
		}
		d := obj(f.request("GET", "/product?sort=price_desc&limit=100", "", nil, 200))
		last := float64(1e15)
		for _, v := range list(d["items"]) {
			p := obj(v)["price"].(float64)
			if p > last {
				t.Fatal("not sorted")
			}
			last = p
		}
		for _, q := range []string{"page=0", "page=oops", "limit=101", "sort=bad"} {
			f.request("GET", "/product?"+q, "", nil, 400)
		}
		d = obj(f.request("GET", "/product?"+url.Values{"brand": {"' OR 1=1 --"}}.Encode(), "", nil, 200))
		if d["total"] != float64(0) {
			t.Fatal("SQL injection")
		}
	})
	t.Run("role boundaries", func(t *testing.T) {
		for _, path := range []string{"/product", "/photo", "/role", "/cart", "/order", "/order_item"} {
			f.request("POST", path, "", map[string]any{}, 401)
		}
		for _, path := range []string{"/user", "/role", "/user/role/00000000-0000-4000-8000-000000000001"} {
			f.request("GET", path, client, nil, 403)
		}
		f.request("POST", "/product", manager, map[string]any{}, 403)
		f.request("POST", "/cart", manager, map[string]any{}, 403)
		f.request("GET", "/cart/"+managerID, client, nil, 403)
		f.request("POST", "/user", "", map[string]any{"name": "Escalation", "email": "evil@example.com", "password": "Password123!", "password_confirmation": "Password123!", "phone": "+79000000000", "privacy_accepted": true, "role_id": "00000000-0000-4000-8000-000000000003"}, 403)
		f.request("PUT", "/user/"+clientID, client, map[string]any{"name": "Client", "email": "client@styleshop.local", "phone": "+79000000000", "role_id": "00000000-0000-4000-8000-000000000003"}, 403)
		r := httptest.NewRequest("POST", "/order", strings.NewReader("{}"))
		r.Header.Set("Origin", "https://other.example")
		r.Header.Set("Authorization", "Bearer "+client)
		w := httptest.NewRecorder()
		f.app.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	})
	var productID string
	product := map[string]any{"name": "TEST TEE", "type": "Футболки", "gender": "Унисекс", "size": "M", "color": "Чёрный", "description": "Test cotton tee", "brand": "StyleShop", "season": "Лето", "price": 250000, "image_index": 1}
	t.Run("product create update detail", func(t *testing.T) {
		p := obj(f.request("POST", "/product", admin, product, 201))
		productID = p["id"].(string)
		if len(productID) != 36 {
			t.Fatal(productID)
		}
		f.request("GET", "/product/"+productID, "", nil, 200)
		product["price"] = 260000
		f.request("PUT", "/product/"+productID, admin, product, 200)
		product["price"] = -1
		f.request("PUT", "/product/"+productID, admin, product, 400)
		product["price"] = 260000
		f.request("PUT", "/product/"+productID, client, product, 403)
	})
	t.Run("photo upload download and delete", func(t *testing.T) {
		var imageBytes bytes.Buffer
		if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
			t.Fatal(err)
		}
		var body bytes.Buffer
		mp := multipart.NewWriter(&body)
		mp.WriteField("product_id", productID)
		part, _ := mp.CreateFormFile("file", "../../photo.png")
		part.Write(imageBytes.Bytes())
		mp.Close()
		r := httptest.NewRequest("POST", "/photo", &body)
		r.Header.Set("Authorization", "Bearer "+admin)
		r.Header.Set("Content-Type", mp.FormDataContentType())
		w := httptest.NewRecorder()
		f.app.ServeHTTP(w, r)
		if w.Code != 201 {
			t.Fatalf("upload: %d %s", w.Code, w.Body.String())
		}
		var p map[string]any
		json.Unmarshal(w.Body.Bytes(), &p)
		id := p["id"].(string)
		if strings.Contains(p["filename"].(string), "/") {
			t.Fatal("unsafe filename")
		}
		r = httptest.NewRequest("GET", "/photo/"+id, nil)
		w = httptest.NewRecorder()
		f.app.ServeHTTP(w, r)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), imageBytes.Bytes()) || w.Header().Get("Content-Type") != "image/png" {
			t.Fatal("image mismatch")
		}
		f.request("DELETE", "/photo/"+id, client, nil, 403)
		f.request("DELETE", "/photo/"+id, admin, nil, 200)
		f.request("GET", "/photo/"+id, "", nil, 404)
		body.Reset()
		mp = multipart.NewWriter(&body)
		mp.WriteField("product_id", productID)
		part, _ = mp.CreateFormFile("file", "x.svg")
		part.Write([]byte("<svg onload='alert(1)'></svg>"))
		mp.Close()
		r = httptest.NewRequest("POST", "/photo", &body)
		r.Header.Set("Authorization", "Bearer "+admin)
		r.Header.Set("Content-Type", mp.FormDataContentType())
		w = httptest.NewRecorder()
		f.app.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal("accepted invalid image")
		}
	})
	var orderID string
	t.Run("cart ownership quantities and atomic checkout", func(t *testing.T) {
		f.request("POST", "/order", client, map[string]any{}, 400)
		f.request("POST", "/cart", client, map[string]any{"product_id": productID, "user_id": managerID, "count": 1}, 403)
		f.request("POST", "/cart", client, map[string]any{"product_id": productID, "count": 0}, 400)
		c := obj(f.request("POST", "/cart", client, map[string]any{"product_id": productID, "count": 1}, 201))
		cartID := c["id"].(string)
		f.request("POST", "/cart", client, map[string]any{"product_id": productID, "count": 1}, 201)
		f.request("PUT", "/cart/"+cartID, client, map[string]int{"count": 3}, 200)
		f.request("PUT", "/cart/"+cartID, client, map[string]int{"count": 100}, 400)
		cart := obj(f.request("GET", "/cart/"+clientID, client, nil, 200))
		if cart["total"] != float64(780000) {
			t.Fatal(cart)
		}
		o := obj(f.request("POST", "/order", client, map[string]any{}, 201))
		orderID = o["id"].(string)
		if o["total"] != float64(780000) || o["status"] != "Принят" {
			t.Fatal(o)
		}
		cart = obj(f.request("GET", "/cart/"+clientID, client, nil, 200))
		if len(list(cart["items"])) != 0 {
			t.Fatal("cart not cleared")
		}
		f.request("POST", "/order", client, map[string]any{}, 400)
		f.request("POST", "/cart", client, map[string]any{"product_id": productID, "count": 1}, 201)
		c = obj(f.request("GET", "/cart/"+clientID, client, nil, 200))
		cartID = obj(list(c["items"])[0])["id"].(string)
		f.request("DELETE", "/cart/"+cartID, client, nil, 200)
	})
	t.Run("order privacy items filters and status workflow", func(t *testing.T) {
		u := obj(f.request("POST", "/user", "", map[string]any{"name": "Other", "email": "other@example.com", "password": "Password123!", "password_confirmation": "Password123!", "phone": "+79000000000", "privacy_accepted": true}, 201))
		token := obj(f.request("POST", "/auth/login", "", map[string]string{"email": "other@example.com", "password": "Password123!"}, 200))["token"].(string)
		f.request("GET", "/order/"+orderID, token, nil, 404)
		f.request("GET", "/order_item/"+orderID, token, nil, 404)
		f.request("GET", "/cart/"+clientID, token, nil, 403)
		foreignCart := obj(f.request("POST", "/cart", token, map[string]any{"product_id": productID, "count": 1}, 201))
		f.request("PUT", "/cart/"+foreignCart["id"].(string), client, map[string]int{"count": 2}, 404)
		f.request("DELETE", "/cart/"+foreignCart["id"].(string), client, nil, 404)
		d := obj(f.request("GET", "/order", token, nil, 200))
		if d["total"] != float64(0) {
			t.Fatal("foreign orders leaked")
		}
		f.request("DELETE", "/user/"+u["id"].(string), admin, nil, 200)
		items := list(f.request("GET", "/order_item/"+orderID, client, nil, 200))
		if len(items) != 1 || obj(items[0])["count"] != float64(3) {
			t.Fatal(items)
		}
		f.request("POST", "/order_item", client, map[string]any{}, 403)
		f.request("POST", "/order_item", admin, map[string]any{"order_id": orderID, "product_id": productID, "price": 1}, 400)
		f.request("POST", "/order_item", admin, map[string]any{"order_id": orderID, "product_id": productID, "count": 1}, 201)
		f.request("PUT", "/order/"+orderID, client, map[string]string{"status": "В работе"}, 403)
		f.request("PUT", "/order/"+orderID, manager, map[string]string{"status": "Собран"}, 200)
		for _, status := range []string{"В работе", "Собран", "Готов к выдаче"} {
			f.request("PUT", "/order/"+orderID, manager, map[string]string{"status": status}, 200)
		}
		f.request("PUT", "/order/"+orderID, admin, map[string]string{"status": "Отменён"}, 400)
		d = obj(f.request("GET", "/order?"+url.Values{"status": {"Готов к выдаче"}, "from": {"2020-01-01T00:00:00Z"}, "to": {"2099-12-31T23:59:59Z"}, "page": {"1"}, "limit": {"1"}}.Encode(), manager, nil, 200))
		if d["total"] != float64(1) {
			t.Fatal(d)
		}
		f.request("GET", "/order?from=bad", client, nil, 400)
		f.request("GET", "/order?status=bad", admin, nil, 400)
	})
	t.Run("historical prices survive product changes and deletion", func(t *testing.T) {
		product["price"] = 999999
		f.request("PUT", "/product/"+productID, admin, product, 200)
		o := obj(f.request("GET", "/order/"+orderID, client, nil, 200))
		if o["total"] != float64(1040000) {
			t.Fatal("historical total changed", o)
		}
		f.request("DELETE", "/product/"+productID, admin, nil, 200)
		f.request("GET", "/product/"+productID, "", nil, 404)
		f.request("GET", "/order/"+orderID, client, nil, 200)
		f.request("POST", "/cart", client, map[string]any{"product_id": productID, "count": 1}, 404)
	})
	t.Run("user and custom role CRUD", func(t *testing.T) {
		r := obj(f.request("POST", "/role", admin, map[string]string{"name": "auditor"}, 201))
		rid := r["id"].(string)
		f.request("GET", "/role", admin, nil, 200)
		f.request("PUT", "/role/"+rid, admin, map[string]string{"name": "observer"}, 200)
		f.request("DELETE", "/role/00000000-0000-4000-8000-000000000003", admin, nil, 400)
		f.request("PUT", "/role/00000000-0000-4000-8000-000000000003", admin, map[string]string{"name": "broken"}, 400)
		u := obj(f.request("POST", "/user", admin, map[string]any{"name": "Staff", "email": "staff@example.com", "password": "Password123!", "password_confirmation": "Password123!", "phone": "+79000000000", "privacy_accepted": true, "role_id": rid}, 201))
		uid := u["id"].(string)
		if _, ok := u["password_hash"]; ok {
			t.Fatal("hash leaked")
		}
		f.request("GET", "/user?page=1&limit=2", admin, nil, 200)
		data := obj(f.request("GET", "/user/role/"+rid, admin, nil, 200))
		if data["total"] != float64(1) {
			t.Fatal(data)
		}
		f.request("PUT", "/user/"+uid, admin, map[string]string{"name": "Updated", "email": "staff@example.com", "phone": "+79000000000", "role_id": rid}, 200)
		f.request("DELETE", "/role/"+rid, admin, nil, 409)
		f.request("DELETE", "/user/"+uid, admin, nil, 200)
		f.request("DELETE", "/role/"+rid, admin, nil, 200)
		f.request("PUT", "/user/10000000-0000-4000-8000-000000000003", admin, map[string]string{"name": "Admin", "email": "admin@styleshop.local", "phone": "+79000000000", "role_id": "00000000-0000-4000-8000-000000000001"}, 400)
		f.request("DELETE", "/user/"+clientID, admin, nil, 409)
	})
	t.Run("logout and invalid session", func(t *testing.T) {
		f.request("GET", "/auth/me", client, nil, 200)
		f.request("POST", "/auth/logout", client, map[string]any{}, 200)
		f.request("GET", "/auth/me", client, nil, 401)
		f.request("GET", "/auth/me", "invalid-token", nil, 401)
	})
}

func TestConcurrentCheckoutDoesNotDuplicateOrder(t *testing.T) {
	f := newFixture(t)
	token := f.login("client")
	p := obj(list(obj(f.request("GET", "/product", "", nil, 200))["items"])[0])
	f.request("POST", "/cart", token, map[string]any{"product_id": p["id"], "count": 1}, 201)
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/order", strings.NewReader("{}"))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			f.app.ServeHTTP(w, r)
			codes <- w.Code
		}()
	}
	wg.Wait()
	close(codes)
	got := []int{}
	for c := range codes {
		got = append(got, c)
	}
	sort.Ints(got)
	if got[0] != 201 || got[1] != 400 {
		t.Fatal(got)
	}
	var n int
	if err := f.db.QueryRow("SELECT count(*) FROM orders").Scan(&n); err != nil || n != 1 {
		t.Fatalf("orders=%d error=%v", n, err)
	}
}

func TestPasswordChangeRevokesSessions(t *testing.T) {
	f := newFixture(t)
	token := f.login("client")
	f.request("PUT", "/user/10000000-0000-4000-8000-000000000001", token, map[string]string{"name": "Client", "email": "client@styleshop.local", "phone": "+79000000000", "password": "AnotherPassword2026!", "password_confirmation": "AnotherPassword2026!"}, 200)
	f.request("GET", "/auth/me", token, nil, 401)
	f.request("POST", "/auth/login", "", map[string]string{"email": "client@styleshop.local", "password": "StyleDemo2026!"}, 401)
	f.request("POST", "/auth/login", "", map[string]string{"email": "client@styleshop.local", "password": "AnotherPassword2026!"}, 200)
}

func TestStaticFilesAndSessionCookie(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{"/", "/style.css", "/app.js", "/assets/styleshop/2b4e9455-c6c9-4a8d-a1eb-0c297c41a83e.jpg"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		f.app.ServeHTTP(w, r)
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatal(path, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"email":"client@styleshop.local","password":"StyleDemo2026!"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.app.ServeHTTP(w, r)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("insecure cookie")
	}
	r = httptest.NewRequest("GET", "/auth/me", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	f.app.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}
