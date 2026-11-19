package httpapi

import (
	"clothing-store/internal/store"
	"embed"
	"os"
	"path/filepath"
	"time"
)

//go:embed testdata/*.png
var catalogFixturePhotos embed.FS

func (a *App) seedCatalogPhotos() error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	const marker = "catalog_hoodie_photos_v1"
	var done int
	if err = tx.QueryRow("SELECT count(*) FROM schema_migrations WHERE name=?", marker).Scan(&done); err != nil {
		return err
	}
	if done != 0 {
		return nil
	}
	rows, err := tx.Query("SELECT id FROM products WHERE name=? AND brand=? AND deleted=0 ORDER BY rowid", "Худи «Уголь»", "StyleShop")
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	for _, asset := range []string{"hoodie-front.png", "hoodie-detail.png"} {
		data, e := catalogFixturePhotos.ReadFile("testdata/" + asset)
		if e != nil {
			return e
		}
		filename := "catalog-" + asset
		if err = os.WriteFile(filepath.Join(a.cfg.UploadDir, filename), data, 0600); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err = tx.Exec("INSERT INTO photos(id,filename,product_id,mime_type) VALUES(?,?,?,?)", store.ID(), filename, id, "image/png"); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec("INSERT INTO schema_migrations(name,applied_at) VALUES(?,?)", marker, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}
