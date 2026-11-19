package store_test

import (
	"clothing-store/internal/store"
	"clothing-store/internal/testsupport"
	"clothing-store/migrations"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSeasonCategoriesUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seasons.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = testsupport.Seed(db, "StyleDemo2026!"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		season, attributes, want string
	}{
		{"Демисезонный", `{"brand":["Марка"]}`, "Демисезон"},
		{"Лето, Демисезонный", `{"season":["Лето","Демисезонный","Осень","осень"],"brand":["Марка"]}`, "Лето, Демисезон"},
		{"Всесезон", `{"brand":["Марка"]}`, "Лето, Зима, Демисезон"},
		{"Зима", `{"season":["Зима"],"brand":["Марка"]}`, "Зима"},
		{"Межсезонье", `{"season":["Межсезонье"],"brand":["Марка"]}`, "Демисезон"},
	}
	rows, err := db.Query("SELECT id FROM products ORDER BY rowid LIMIT ?", len(cases))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for i, tc := range cases {
		if _, err = db.Exec("UPDATE products SET season=?,attributes=? WHERE id=?", tc.season, tc.attributes, ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec("DELETE FROM schema_migrations WHERE name IN ('009_calendar_seasons.sql','010_season_categories.sql')"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	for attempt := 0; attempt < 2; attempt++ {
		db, err = store.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		for i, tc := range cases {
			var season, attributes string
			if err = db.QueryRow("SELECT season,attributes FROM products WHERE id=?", ids[i]).Scan(&season, &attributes); err != nil || season != tc.want {
				t.Fatal(season, tc.want, err)
			}
			var attrs map[string][]string
			if err = json.Unmarshal([]byte(attributes), &attrs); err != nil || len(attrs["brand"]) != 1 || attrs["brand"][0] != "Марка" {
				t.Fatal(attributes, err)
			}
			for _, value := range attrs["season"] {
				if value != "Демисезон" && value != "Лето" && value != "Зима" {
					t.Fatal(attrs)
				}
			}
		}
		db.Close()
	}
}

func TestPersistenceAndIdempotentSeed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = testsupport.Seed(db, "StyleDemo2026!"); err != nil {
		t.Fatal(err)
	}
	if err = testsupport.Seed(db, "StyleDemo2026!"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM products").Scan(&n); err != nil || n != 18 {
		t.Fatalf("products=%d error=%v", n, err)
	}
	if _, err = db.Exec("UPDATE products SET price=123456 WHERE rowid=(SELECT min(rowid) FROM products)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.QueryRow("SELECT price FROM products ORDER BY rowid LIMIT 1").Scan(&n); err != nil || n != 123456 {
		t.Fatalf("price=%d error=%v", n, err)
	}
	if err = db.QueryRow("SELECT count(*) FROM roles").Scan(&n); err != nil || n != 3 {
		t.Fatal(n, err)
	}
}

func TestUpgradeExistingDatabasePreservesOrders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_schema.sql", "002_roles.sql"} {
		b, e := migrations.Files.ReadFile(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(b)); e != nil {
			t.Fatal(e)
		}
	}
	if err = testsupport.Seed(db, "StyleDemo2026!"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO orders(id,user_id,date_time,status) VALUES('old-order','10000000-0000-4000-8000-000000000001','2026-11-16T00:00:00Z','В работе')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	for i := 0; i < 2; i++ {
		db, err = store.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var status, reason string
		if err = db.QueryRow("SELECT status,cancellation_reason FROM orders WHERE id='old-order'").Scan(&status, &reason); err != nil || status != "В работе" || reason != "" {
			t.Fatal(status, reason, err)
		}
		var number int
		if err = db.QueryRow("SELECT order_number FROM orders WHERE id='old-order'").Scan(&number); err != nil || number != 1 {
			t.Fatal("existing order number", number, err)
		}
		var n int
		if err = db.QueryRow("SELECT count(*) FROM schema_migrations WHERE name='003_order_cancellation.sql'").Scan(&n); err != nil || n != 1 {
			t.Fatal(n, err)
		}
		db.Close()
	}
}
