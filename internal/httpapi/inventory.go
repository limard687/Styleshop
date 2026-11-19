package httpapi

import (
	"database/sql"
	"net/http"
)

func reserveStock(tx *sql.Tx, id string, count int) error {
	result, err := tx.Exec("UPDATE products SET stock=stock-? WHERE id=? AND deleted=0 AND stock>=?", count, id, count)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return bad("Недостаточно товара на складе. Обновите корзину и уменьшите количество.")
	}
	return nil
}

func (a *App) updateStock(w http.ResponseWriter, r *http.Request) error {
	var v struct {
		Stock         *int `json:"stock"`
		ExpectedStock *int `json:"expected_stock"`
	}
	if err := decode(w, r, &v); err != nil {
		return err
	}
	if v.Stock == nil || v.ExpectedStock == nil || *v.Stock < 0 || *v.Stock > 1000000 {
		return bad("Укажите остаток от 0 до 1000000 и предыдущий остаток")
	}
	result, err := a.db.Exec("UPDATE products SET stock=? WHERE id=? AND deleted=0 AND stock=?", *v.Stock, r.PathValue("id"), *v.ExpectedStock)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return apiError{409, "Остаток изменился. Обновите склад перед сохранением."}
	}
	p, err := a.one(productSelect+" WHERE p.id=?", r.PathValue("id"))
	if err != nil {
		return err
	}
	return respond(w, 200, p)
}
