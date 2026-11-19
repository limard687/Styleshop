package httpapi

import "testing"

func TestCatalogPhotosSeedOnce(t *testing.T) {
	f := newFixture(t)
	for attempt := 0; attempt < 2; attempt++ {
		if err := f.app.seedCatalogPhotos(); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := f.db.QueryRow("SELECT count(*) FROM photos").Scan(&count); err != nil || count != 6 {
		t.Fatal(count, err)
	}
	products := obj(f.request("GET", "/product?q=Уголь", "", nil, 200))
	for _, value := range list(products["items"]) {
		p := obj(value)
		detail := obj(f.request("GET", "/product/"+p["id"].(string), "", nil, 200))
		if len(list(detail["photos"])) != 2 {
			t.Fatal(detail)
		}
	}
	if _, err := f.db.Exec("DELETE FROM photos"); err != nil {
		t.Fatal(err)
	}
	if err := f.app.seedCatalogPhotos(); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("SELECT count(*) FROM photos").Scan(&count); err != nil || count != 0 {
		t.Fatal("deleted photos restored", count, err)
	}
}
