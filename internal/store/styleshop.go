package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"clothing-store/web"
	"golang.org/x/crypto/bcrypt"
)

// DemoCatalog is the original StyleShop catalog. Prices in its source are rubles.
type DemoCatalog struct {
	Products []struct {
		ID, Name, Type, Gender, Size, Color, Description, Brand, Season string
		Price                                                           float64
		Stock                                                           int
	} `json:"products"`
	Photos []struct {
		ID, Filename string
		ProductID    string `json:"product_id"`
	} `json:"photos"`
}

func ReadDemoCatalog() (DemoCatalog, error) {
	var catalog DemoCatalog
	b, err := web.Files.ReadFile("static/assets/styleshop/catalog.json")
	if err != nil {
		return catalog, err
	}
	err = json.Unmarshal(b, &catalog)
	return catalog, err
}

// SeedStyleShop fills a new database without replacing edited products or accounts.
func SeedStyleShop(db *sql.DB, password string) error {
	catalog, err := ReadDemoCatalog()
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, role := range []string{"client", "manager", "admin"} {
		_, err = tx.Exec(`INSERT OR IGNORE INTO users(id,name,email,phone,role_id,registration_date,password_hash) VALUES(?,?,?,?,?,?,?)`, fmt.Sprintf("10000000-0000-4000-8000-%012d", i+1), []string{"Покупатель", "Менеджер", "Администратор"}[i], role+"@styleshop.local", "", fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1), time.Now().UTC().Format(time.RFC3339), string(hash))
		if err != nil {
			return err
		}
	}
	var done int
	if err = tx.QueryRow("SELECT count(*) FROM schema_migrations WHERE name='styleshop_catalog_v1'").Scan(&done); err != nil {
		return err
	}
	if done == 0 {
		var count int
		if err = tx.QueryRow("SELECT count(*) FROM products").Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			for _, p := range catalog.Products {
				season := p.Season
				switch season {
				case "Осень", "осень", "весна", "Весна":
					season = "Демисезон"
				case "всесезон":
					season = "Круглогодичный"
				case "лето":
					season = "Лето"
				}
				gender := p.Gender
				switch gender {
				case "мужской":
					gender = "Мужской"
				case "женский":
					gender = "Женский"
				case "унисекс":
					gender = "Унисекс"
				}
				attrs, err := json.Marshal(map[string][]string{"type": {p.Type}, "gender": {gender}, "size": {p.Size}, "color": {p.Color}, "brand": {p.Brand}, "season": {season}})
				if err != nil {
					return err
				}
				_, err = tx.Exec(`INSERT INTO products(id,name,type,gender,size,color,description,brand,season,price,image_index,stock,attributes) VALUES(?,?,?,?,?,?,?,?,?,?,-1,?,?)`, p.ID, p.Name, p.Type, gender, p.Size, p.Color, p.Description, p.Brand, season, int64(p.Price*100+0.5), p.Stock, string(attrs))
				if err != nil {
					return err
				}
			}
		}
		if _, err = tx.Exec("INSERT INTO schema_migrations(name,applied_at) VALUES('styleshop_catalog_v1',?)", time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
