package httpapi

import (
	"mime"
	"os"
	"path/filepath"
	"time"

	"clothing-store/internal/store"
	"clothing-store/web"
)

func (a *App) seedStyleShopPhotos() error {
	catalog, err := store.ReadDemoCatalog()
	if err != nil {
		return err
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var done int
	if err = tx.QueryRow("SELECT count(*) FROM schema_migrations WHERE name='styleshop_photos_v1'").Scan(&done); err != nil {
		return err
	}
	if done != 0 {
		return nil
	}
	for _, p := range catalog.Photos {
		var exists int
		if err = tx.QueryRow("SELECT count(*) FROM products WHERE id=? AND deleted=0", p.ProductID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			continue
		}
		b, err := web.Files.ReadFile("static/assets/styleshop/" + p.Filename)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(a.cfg.UploadDir, p.Filename), b, 0600); err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT OR IGNORE INTO photos(id,filename,product_id,mime_type) VALUES(?,?,?,?)", p.ID, p.Filename, p.ProductID, mime.TypeByExtension(filepath.Ext(p.Filename))); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("INSERT INTO schema_migrations(name,applied_at) VALUES('styleshop_photos_v1',?)", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}
