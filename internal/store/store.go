package store

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clothing-store/migrations"
	"github.com/mattn/go-sqlite3"
)

func init() {
	sql.Register("form_sqlite3", &sqlite3.SQLiteDriver{ConnectHook: func(conn *sqlite3.SQLiteConn) error {
		return conn.RegisterFunc("casefold", strings.ToLower, true)
	}})
}

func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("form_sqlite3", path+"?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err = migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, name := range []string{"001_schema.sql", "002_roles.sql"} {
		b, e := migrations.Files.ReadFile(name)
		if e != nil {
			return e
		}
		if _, err = tx.Exec(string(b)); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	for _, name := range []string{"003_order_cancellation.sql", "004_catalog_privacy.sql", "008_product_attributes.sql", "009_calendar_seasons.sql", "010_season_categories.sql", "011_order_numbers.sql", "012_inventory.sql"} {
		var n int
		if err = tx.QueryRow("SELECT count(*) FROM schema_migrations WHERE name=?", name).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			continue
		}
		b, err := migrations.Files.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(string(b)); err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO schema_migrations(name,applied_at) VALUES(?,?)", name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
