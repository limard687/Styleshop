package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSelectedCheckoutAndCancellation(t *testing.T) {
	f := newFixture(t)
	client, manager, admin := f.login("client"), f.login("manager"), f.login("admin")
	const clientID = "10000000-0000-4000-8000-000000000001"
	products := list(obj(f.request("GET", "/product", "", nil, 200))["items"])
	p1, p2 := obj(products[0]), obj(products[1])
	c1 := obj(f.request("POST", "/cart", client, map[string]any{"product_id": p1["id"], "count": 2}, 201))["id"].(string)
	c2 := obj(f.request("POST", "/cart", client, map[string]any{"product_id": p2["id"], "count": 1}, 201))["id"].(string)

	for _, ids := range [][]string{{}, {c1, c1}, {""}} {
		f.request("POST", "/order", client, map[string]any{"cart_ids": ids}, 400)
	}
	f.request("POST", "/order", client, map[string]any{"cart_ids": []string{c1, "missing"}}, 404)
	var count int
	if err := f.db.QueryRow("SELECT count(*) FROM orders").Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid selection created an order", count, err)
	}

	other := obj(f.request("POST", "/user", "", map[string]any{"name": "Other", "email": "other@example.com", "password": "Password123!", "password_confirmation": "Password123!", "phone": "+79000000000", "privacy_accepted": true}, 201))
	otherToken := obj(f.request("POST", "/auth/login", "", map[string]string{"email": "other@example.com", "password": "Password123!"}, 200))["token"].(string)
	foreignCart := obj(f.request("POST", "/cart", otherToken, map[string]any{"product_id": p1["id"], "count": 1}, 201))["id"].(string)
	f.request("POST", "/order", client, map[string]any{"cart_ids": []string{foreignCart}}, 404)
	f.request("POST", "/order", client, map[string]any{"cart_ids": []string{c1, foreignCart}}, 404)

	o := obj(f.request("POST", "/order", client, map[string]any{"cart_ids": []string{c1}}, 201))
	id := o["id"].(string)
	if o["total"] != p1["price"].(float64)*2 {
		t.Fatal(o)
	}
	items := list(f.request("GET", "/order_item/"+id, client, nil, 200))
	if len(items) != 1 || obj(items[0])["product_id"] != p1["id"] || obj(items[0])["count"] != float64(2) {
		t.Fatal(items)
	}
	for _, token := range []string{client, manager, admin} {
		detail := obj(f.request("GET", "/order/"+id, token, nil, 200))
		if len(list(detail["items"])) != 1 {
			t.Fatal("order contents missing", detail)
		}
	}
	cart := obj(f.request("GET", "/cart/"+clientID, client, nil, 200))
	if len(list(cart["items"])) != 1 || obj(list(cart["items"])[0])["id"] != c2 {
		t.Fatal("unselected item was removed", cart)
	}
	f.request("POST", "/order", client, map[string]any{"cart_ids": []string{c1}}, 404)

	for _, token := range []string{manager, admin} {
		for _, reason := range []string{"", " \n\t", strings.Repeat("я", 1001)} {
			f.request("PUT", "/order/"+id, token, map[string]string{"status": "Отменён", "cancellation_reason": reason}, 400)
		}
	}
	f.request("PUT", "/order/"+id, manager, map[string]string{"status": "В работе", "cancellation_reason": "not cancellation"}, 400)
	f.request("PUT", "/order/"+id, client, map[string]string{"status": "В работе"}, 403)
	f.request("PUT", "/order/"+id, otherToken, map[string]string{"status": "Отменён"}, 404)
	f.request("PUT", "/order/"+id, manager, map[string]string{"status": "Отменён", "cancellation_reason": "  Товар повреждён при сборке.\nПокупатель отказался от замены.  "}, 200)
	for _, token := range []string{client, manager, admin} {
		detail := obj(f.request("GET", "/order/"+id, token, nil, 200))
		if detail["status"] != "Отменён" || detail["cancellation_reason"] != "Товар повреждён при сборке.\nПокупатель отказался от замены." || detail["cancellation_actor"] != "manager" || detail["cancelled_at"] == "" {
			t.Fatal(detail)
		}
		if len(list(detail["items"])) != 1 {
			t.Fatal("cancellation erased items")
		}
	}
	f.request("PUT", "/order/"+id, admin, map[string]string{"status": "Отменён", "cancellation_reason": "Rewrite history"}, 400)
	f.request("PUT", "/order/"+id, admin, map[string]string{"status": "В работе"}, 400)
	f.request("PUT", "/order/"+id, manager, map[string]string{"status": "Отменён"}, 200)

	o = obj(f.request("POST", "/order", client, map[string]any{}, 201))
	id = o["id"].(string)
	f.request("PUT", "/order/"+id, manager, map[string]string{"status": "В работе"}, 200)
	cancelled := obj(f.request("PUT", "/order/"+id, client, map[string]string{"status": "Отменён"}, 200))
	if cancelled["cancellation_actor"] != "client" || cancelled["cancellation_reason"] != "Отменён покупателем" || cancelled["cancelled_by"] != clientID {
		t.Fatal(cancelled)
	}

	f.request("POST", "/cart", client, map[string]any{"product_id": p1["id"], "count": 1}, 201)
	id = obj(f.request("POST", "/order", client, map[string]any{}, 201))["id"].(string)
	for _, status := range []string{"В работе", "Собран", "Готов к выдаче"} {
		f.request("PUT", "/order/"+id, manager, map[string]string{"status": status}, 200)
		if status == "Собран" {
			f.request("PUT", "/order/"+id, client, map[string]string{"status": "Отменён"}, 400)
		}
	}
	f.request("PUT", "/order/"+id, client, map[string]string{"status": "Отменён", "cancellation_reason": "Too late"}, 400)
	f.request("PUT", "/order/"+id, manager, map[string]string{"status": "Отменён", "cancellation_reason": "Отмена после готовности"}, 200)
	f.request("POST", "/cart", client, map[string]any{"product_id": p1["id"], "count": 1}, 201)
	id = obj(f.request("POST", "/order", client, map[string]any{}, 201))["id"].(string)
	f.request("PUT", "/order/"+id, admin, map[string]string{"status": "Готов к выдаче"}, 200)
	f.request("PUT", "/order/"+id, admin, map[string]string{"status": "Отменён", "cancellation_reason": "Отмена администратором после готовности"}, 200)
	f.request("DELETE", "/user/"+other["id"].(string), admin, nil, 200)
}

func TestMultiplePhotosAndAtomicUpload(t *testing.T) {
	f := newFixture(t)
	admin := f.login("admin")
	product := obj(list(obj(f.request("GET", "/product", "", nil, 200))["items"])[0])
	pid := product["id"].(string)
	var valid bytes.Buffer
	if err := png.Encode(&valid, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	upload := func(files [][]byte, want int) map[string]any {
		t.Helper()
		var body bytes.Buffer
		mp := multipart.NewWriter(&body)
		mp.WriteField("product_id", pid)
		for _, data := range files {
			part, err := mp.CreateFormFile("file", "photo.png")
			if err != nil {
				t.Fatal(err)
			}
			part.Write(data)
		}
		mp.Close()
		r := httptest.NewRequest("POST", "/photo", &body)
		r.Header.Set("Authorization", "Bearer "+admin)
		r.Header.Set("Content-Type", mp.FormDataContentType())
		w := httptest.NewRecorder()
		f.app.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("upload got %d want %d: %s", w.Code, want, w.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	batch := upload([][]byte{valid.Bytes(), valid.Bytes()}, 201)
	if batch["total"] != float64(2) {
		t.Fatal(batch)
	}
	photos := list(obj(f.request("GET", "/product/"+pid, "", nil, 200))["photos"])
	if len(photos) != 2 {
		t.Fatal(photos)
	}
	for i, value := range photos {
		photo := obj(value)
		if photo["id"] != obj(list(batch["items"])[i])["id"] {
			t.Fatal("upload order not preserved")
		}
		r := httptest.NewRequest("GET", "/photo/"+photo["id"].(string), nil)
		w := httptest.NewRecorder()
		f.app.ServeHTTP(w, r)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), valid.Bytes()) {
			t.Fatal("photo not retrievable")
		}
	}
	upload([][]byte{valid.Bytes(), []byte("not an image")}, 400)
	upload(make([][]byte, 11), 400)
	tooLarge := append(append([]byte{}, valid.Bytes()...), make([]byte, 8<<20)...)
	upload([][]byte{tooLarge}, 400)
	photos = list(obj(f.request("GET", "/product/"+pid, "", nil, 200))["photos"])
	files, err := os.ReadDir(f.app.cfg.UploadDir)
	if err != nil || len(photos) != 2 || len(files) != 2 {
		t.Fatal("partial upload left data behind", len(photos), len(files), err)
	}
	f.request("DELETE", "/photo/"+obj(photos[0])["id"].(string), admin, nil, 200)
	photos = list(obj(f.request("GET", "/product/"+pid, "", nil, 200))["photos"])
	if len(photos) != 1 {
		t.Fatal(photos)
	}
}
