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
	"testing"
)

func TestMultiFiltersAndBrandSearch(t *testing.T) {
	f := newFixture(t)
	admin := f.login("admin")
	for _, v := range []struct{ gender, season, brand string }{
		{"Мужской", "Зима", "Тихий Бренд"},
		{"Женский", "Лето", "StyleShop Studio"},
		{"Унисекс", "Демисезон", "Another"},
	} {
		f.request("POST", "/product", admin, map[string]any{"name": v.brand, "type": "Худи", "gender": v.gender, "size": "M", "color": "Чёрный", "description": "Описание", "brand": v.brand, "season": v.season, "price": 10000}, 201)
	}
	for _, test := range []struct {
		query url.Values
		total float64
	}{
		{url.Values{"gender": {"Мужской", "Женский"}, "season": {"Зима", "Лето"}}, 2},
		{url.Values{"gender": {"Мужской", "Женский"}, "season": {"Демисезон"}}, 9},
		{url.Values{"brand": {"бренд"}}, 1},
		{url.Values{"brand": {"БРЕНД"}}, 1},
		{url.Values{"brand": {" studio "}}, 1},
		{url.Values{"brand": {"sTyLeShOp"}}, 19},
		{url.Values{"brand": {"Another", "тихий"}}, 2},
		{url.Values{"type": {"Брюки", "Рубашки"}, "size": {"S", "M"}, "color": {"Чёрный", "Меловой"}, "gender": {"Мужской", "Женский"}, "season": {"Демисезон"}, "brand": {"StyleShop", "Another"}}, 4},
		{url.Values{"q": {"тихий"}}, 1},
		{url.Values{}, 21},
	} {
		data := obj(f.request("GET", "/product?"+test.query.Encode(), "", nil, 200))
		if data["total"] != test.total {
			t.Fatalf("%v: %v", test.query, data)
		}
	}
}

func TestRegistrationRequiresPhoneAndConsent(t *testing.T) {
	f := newFixture(t)
	base := map[string]any{"name": "Новый покупатель", "email": "new@example.com", "password": "Password123!", "password_confirmation": "Password123!", "privacy_accepted": true}
	for _, phone := range []string{"", "    ", "123", "call me please", "+790000000000000000"} {
		base["phone"] = phone
		f.request("POST", "/user", "", base, 400)
	}
	base["phone"] = "+7 (900) 000-00-00"
	base["privacy_accepted"] = false
	f.request("POST", "/user", "", base, 400)
	base["privacy_accepted"] = true
	u := obj(f.request("POST", "/user", "", base, 201))
	if u["role"] != "client" {
		t.Fatal(u)
	}
	var date, version, phone string
	if err := f.db.QueryRow("SELECT privacy_accepted_at,privacy_version,phone FROM users WHERE id=?", u["id"]).Scan(&date, &version, &phone); err != nil {
		t.Fatal(err)
	}
	if date == "" || version != "2026-11-16.2" || phone != "+79000000000" {
		t.Fatal(date, version, phone)
	}
}

func TestCreateProductWithMultipleImagesIsAtomic(t *testing.T) {
	f := newFixture(t)
	admin := f.login("admin")
	var valid bytes.Buffer
	if err := png.Encode(&valid, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	p := map[string]any{"name": "NEW ITEM", "type": "Футболки", "gender": "Женский", "size": "M", "color": "Серый", "description": "Описание", "brand": "StyleShop", "season": "Лето", "price": 10000}
	metadata, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	upload := func(files [][]byte, want int) map[string]any {
		t.Helper()
		var body bytes.Buffer
		mp := multipart.NewWriter(&body)
		mp.WriteField("product", string(metadata))
		for _, data := range files {
			part, e := mp.CreateFormFile("file", "photo.png")
			if e != nil {
				t.Fatal(e)
			}
			part.Write(data)
		}
		mp.Close()
		r := httptest.NewRequest("POST", "/product", &body)
		r.Header.Set("Content-Type", mp.FormDataContentType())
		r.Header.Set("Authorization", "Bearer "+admin)
		w := httptest.NewRecorder()
		f.app.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
		var result map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
			t.Fatal(e)
		}
		return result
	}
	upload([][]byte{valid.Bytes(), []byte("invalid")}, 400)
	var n int
	if err = f.db.QueryRow("SELECT count(*) FROM products").Scan(&n); err != nil || n != 18 {
		t.Fatal(n, err)
	}
	files, err := os.ReadDir(f.app.cfg.UploadDir)
	if err != nil || len(files) != 0 {
		t.Fatal("partial files remain", err)
	}
	created := upload([][]byte{valid.Bytes(), valid.Bytes()}, 201)
	if len(list(created["photos"])) != 2 {
		t.Fatal(created)
	}
	detail := obj(f.request("GET", "/product/"+created["id"].(string), "", nil, 200))
	if len(list(detail["photos"])) != 2 {
		t.Fatal(detail)
	}
}
