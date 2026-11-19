package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductAttributesAndVariants(t *testing.T) {
	f := newFixture(t)
	admin := f.login("admin")
	client := f.login("client")
	attrs := map[string][]string{"type": {"Жилеты", "Верхняя одежда"}, "gender": {"Женский", "Детский"}, "size": {"S", "M"}, "color": {"Чёрный", "Белый"}, "brand": {"StyleShop", "Новая марка"}, "season": {"Лето", "Демисезон", "Круглогодичный"}}
	body := map[string]any{"name": "Жилет", "description": "Описание", "price": 10000, "attributes": attrs}
	p := obj(f.request("POST", "/product", admin, body, 201))
	ids := list(p["variant_ids"])
	if len(ids) != 4 {
		t.Fatal(p)
	}
	for _, filter := range []url.Values{{"type": {"Верхняя одежда"}}, {"gender": {"Детский"}}, {"season": {"Демисезон"}, "type": {"Жилеты"}}, {"season": {"Круглогодичный"}}, {"brand": {"новая МАРКА"}}, {"q": {"Новая марка"}}} {
		result := obj(f.request("GET", "/product?"+filter.Encode(), "", nil, 200))
		if result["total"] != float64(4) {
			t.Fatal(filter, result)
		}
	}
	filter := url.Values{"gender": {"Женский", "Детский"}, "season": {"Лето", "Демисезон", "Круглогодичный"}, "type": {"Жилеты"}, "size": {"M"}, "color": {"Белый"}}
	result := obj(f.request("GET", "/product?"+filter.Encode(), "", nil, 200))
	if result["total"] != float64(1) {
		t.Fatal(result)
	}
	item := obj(list(result["items"])[0])
	cart := obj(f.request("POST", "/cart", client, map[string]any{"product_id": item["id"], "count": 1}, 201))
	order := obj(f.request("POST", "/order", client, map[string]any{"cart_ids": []string{cart["id"].(string)}}, 201))
	detail := obj(f.request("GET", "/order/"+order["id"].(string), client, nil, 200))
	bought := obj(list(detail["items"])[0])
	if bought["size"] != "M" || bought["color"] != "Белый" {
		t.Fatal(bought)
	}
	options := obj(f.request("GET", "/product/options", "", nil, 200))
	seasons := list(options["season"])
	if len(seasons) != 4 || seasons[0] != "Лето" || seasons[1] != "Зима" || seasons[2] != "Демисезон" || seasons[3] != "Круглогодичный" {
		t.Fatal("unexpected season options", seasons)
	}
	for key, want := range map[string]string{"gender": "Детский", "season": "Демисезон", "type": "Жилеты", "brand": "Новая марка"} {
		found := false
		for _, value := range list(options[key]) {
			if value == want {
				found = true
			}
		}
		if !found {
			t.Fatal(options)
		}
	}
	f.request("PUT", "/product/"+p["id"].(string), admin, body, 200)
	var count int
	f.db.QueryRow("SELECT count(*) FROM products WHERE deleted=0").Scan(&count)
	if count != 22 {
		t.Fatal(count)
	}
	f.request("POST", "/product", admin, body, 409)
	for _, badAttrs := range []map[string][]string{{"type": {}}, {"unknown": {"x"}}, {"size": {strings.Repeat("x", 21)}}, {"gender": {strings.Repeat("я", 121)}}, {"season": {"\n"}}, {"season": {"Весна"}}, {"season": {"Осень"}}, {"season": {"Всесезон"}}, {"season": {"Межсезонье"}}, {"color": {"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}}} {
		invalid := map[string]any{"name": "Плохой товар", "type": "Тип", "gender": "Унисекс", "size": "M", "color": "Цвет", "brand": "Марка", "season": "Лето", "description": "Описание", "price": 1, "attributes": badAttrs}
		f.request("POST", "/product", admin, invalid, 400)
	}
	f.db.QueryRow("SELECT count(*) FROM products WHERE deleted=0").Scan(&count)
	if count != 22 {
		t.Fatal(count)
	}
}

func TestVariantPhotosRemainAvailable(t *testing.T) {
	f := newFixture(t)
	admin := f.login("admin")
	var pixels bytes.Buffer
	png.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	metadata, _ := json.Marshal(map[string]any{"name": "Модель с фотографией", "description": "Описание", "price": 10000, "attributes": map[string][]string{"type": {"Худи"}, "gender": {"Унисекс"}, "size": {"S", "M"}, "color": {"Чёрный"}, "brand": {"StyleShop"}, "season": {"Лето"}}})
	var body bytes.Buffer
	mp := multipart.NewWriter(&body)
	mp.WriteField("product", string(metadata))
	part, _ := mp.CreateFormFile("file", "photo.png")
	part.Write(pixels.Bytes())
	mp.Close()
	req := httptest.NewRequest("POST", "/product", &body)
	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+admin)
	w := httptest.NewRecorder()
	f.app.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var p map[string]any
	json.Unmarshal(w.Body.Bytes(), &p)
	ids := list(p["variant_ids"])
	photoIDs := []string{}
	for _, id := range ids {
		product := obj(f.request("GET", "/product/"+id.(string), "", nil, 200))
		photos := list(product["photos"])
		if len(photos) != 1 {
			t.Fatal(product)
		}
		photoIDs = append(photoIDs, obj(photos[0])["id"].(string))
	}
	var filename string
	f.db.QueryRow("SELECT filename FROM photos WHERE id=?", photoIDs[1]).Scan(&filename)
	f.request("DELETE", "/photo/"+photoIDs[0], admin, nil, 200)
	r := httptest.NewRequest("GET", "/photo/"+photoIDs[1], nil)
	rw := httptest.NewRecorder()
	f.app.ServeHTTP(rw, r)
	if rw.Code != 200 {
		t.Fatal(rw.Code)
	}
	if _, err := os.Stat(filepath.Join(f.app.cfg.UploadDir, filename)); err != nil {
		t.Fatal(err)
	}
	f.request("DELETE", "/photo/"+photoIDs[1], admin, nil, 200)
	if _, err := os.Stat(filepath.Join(f.app.cfg.UploadDir, filename)); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
