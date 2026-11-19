package httpapi

import (
	"clothing-store/internal/store"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type cartInput struct {
	ProductID string `json:"product_id"`
	UserID    string `json:"user_id"`
	Count     int    `json:"count"`
}

func (a *App) addCart(w http.ResponseWriter, r *http.Request) error {
	var v cartInput
	if err := decode(w, r, &v); err != nil {
		return err
	}
	u := current(r)
	if v.UserID != "" && v.UserID != u.ID {
		return deny()
	}
	if v.Count < 1 || v.Count > 99 {
		return bad("Количество: от 1 до 99")
	}
	var exists int
	if err := a.db.QueryRow("SELECT 1 FROM products WHERE id=? AND deleted=0", v.ProductID).Scan(&exists); err != nil {
		return err
	}
	id := store.ID()
	result, err := a.db.Exec(`INSERT INTO carts(id,user_id,product_id,count) SELECT ?,?,?,? WHERE (SELECT stock FROM products WHERE id=? AND deleted=0)>=?+COALESCE((SELECT count FROM carts WHERE user_id=? AND product_id=?),0) ON CONFLICT(user_id,product_id) DO UPDATE SET count=count+excluded.count`, id, u.ID, v.ProductID, v.Count, v.ProductID, v.Count, u.ID, v.ProductID)
	if err != nil {
		return err
	}
	if n, e := result.RowsAffected(); e != nil {
		return e
	} else if n == 0 {
		return bad("Недостаточно товара на складе")
	}
	item, err := a.one("SELECT id,user_id,product_id,count FROM carts WHERE user_id=? AND product_id=?", u.ID, v.ProductID)
	if err != nil {
		return err
	}
	return respond(w, 201, item)
}
func (a *App) cart(w http.ResponseWriter, r *http.Request) error {
	if r.PathValue("user_id") != current(r).ID {
		return deny()
	}
	items, err := rowsJSON(a.db, `SELECT c.id,c.user_id,c.product_id,c.count,p.stock,p.name,p.size,p.color,p.price,p.image_index,(SELECT id FROM photos WHERE product_id=p.id ORDER BY rowid LIMIT 1) AS photo_id FROM carts c JOIN products p ON p.id=c.product_id WHERE c.user_id=? AND p.deleted=0 ORDER BY c.rowid`, current(r).ID)
	if err != nil {
		return err
	}
	var total int64
	for _, v := range items {
		total += v["price"].(int64) * v["count"].(int64)
	}
	return respond(w, 200, map[string]any{"items": items, "total": total})
}
func (a *App) updateCart(w http.ResponseWriter, r *http.Request) error {
	var v struct {
		Count int `json:"count"`
	}
	if err := decode(w, r, &v); err != nil {
		return err
	}
	if v.Count < 1 || v.Count > 99 {
		return bad("Количество: от 1 до 99")
	}
	var available int
	if err := a.db.QueryRow("SELECT p.stock FROM products p JOIN carts c ON c.product_id=p.id WHERE c.id=? AND c.user_id=?", r.PathValue("id"), current(r).ID).Scan(&available); err != nil {
		return err
	}
	if v.Count > available {
		return bad("Недостаточно товара на складе")
	}
	if err := affected(a.db.Exec("UPDATE carts SET count=? WHERE id=? AND user_id=?", v.Count, r.PathValue("id"), current(r).ID)); err != nil {
		return err
	}
	item, err := a.one("SELECT id,user_id,product_id,count FROM carts WHERE id=?", r.PathValue("id"))
	if err != nil {
		return err
	}
	return respond(w, 200, item)
}
func (a *App) deleteCart(w http.ResponseWriter, r *http.Request) error {
	if err := affected(a.db.Exec("DELETE FROM carts WHERE id=? AND user_id=?", r.PathValue("id"), current(r).ID)); err != nil {
		return err
	}
	return respond(w, 200, map[string]bool{"ok": true})
}

const orderSelect = `SELECT o.id,o.order_number,o.user_id,o.date_time,o.status,o.cancellation_reason,o.cancellation_actor,o.cancelled_by,o.cancelled_at,u.name AS user_name,u.email AS user_email,COALESCE((SELECT SUM(price*count) FROM order_items WHERE order_id=o.id),0) AS total FROM orders o JOIN users u ON u.id=o.user_id`

func (a *App) createOrder(w http.ResponseWriter, r *http.Request) error {
	var v struct {
		UserID  string    `json:"user_id"`
		CartIDs *[]string `json:"cart_ids"`
	}
	if err := decode(w, r, &v); err != nil {
		return err
	}
	u := current(r)
	if v.UserID != "" && v.UserID != u.ID {
		return deny()
	}
	selection := map[string]bool{}
	if v.CartIDs != nil {
		if len(*v.CartIDs) == 0 || len(*v.CartIDs) > 100 {
			return bad("Выберите от 1 до 100 позиций корзины")
		}
		for _, id := range *v.CartIDs {
			if id == "" || selection[id] {
				return bad("Неверный или повторяющийся ID позиции корзины")
			}
			selection[id] = true
		}
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	items, err := rowsJSON(tx, `SELECT c.id AS cart_id,c.product_id,c.count,p.price FROM carts c JOIN products p ON p.id=c.product_id WHERE c.user_id=? AND p.deleted=0`, u.ID)
	if err != nil {
		return err
	}
	if v.CartIDs != nil {
		chosen := []map[string]any{}
		for _, item := range items {
			if selection[item["cart_id"].(string)] {
				chosen = append(chosen, item)
			}
		}
		if len(chosen) != len(selection) {
			return missing()
		}
		items = chosen
	}
	if len(items) == 0 {
		return bad("Корзина пуста")
	}
	id := store.ID()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err = tx.Exec("INSERT INTO orders(id,user_id,date_time,status,order_number) VALUES(?,?,?,?,(SELECT COALESCE(MAX(order_number),0)+1 FROM orders))", id, u.ID, now, "Принят"); err != nil {
		return err
	}
	for _, v := range items {
		if err = reserveStock(tx, v["product_id"].(string), int(v["count"].(int64))); err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO order_items(id,order_id,product_id,price,count,reserved_count) VALUES(?,?,?,?,?,?)", store.ID(), id, v["product_id"], v["price"], v["count"], v["count"]); err != nil {
			return err
		}
		if _, err = tx.Exec("DELETE FROM carts WHERE id=? AND user_id=?", v["cart_id"], u.ID); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	o, err := a.one(orderSelect+" WHERE o.id=?", id)
	if err != nil {
		return err
	}
	return respond(w, 201, o)
}
func validStatus(s string) bool {
	for _, v := range []string{"Принят", "В работе", "Собран", "Готов к выдаче", "Отменён"} {
		if s == v {
			return true
		}
	}
	return false
}
func (a *App) orders(w http.ResponseWriter, r *http.Request) error {
	p, l, err := pagination(r)
	if err != nil {
		return err
	}
	where := " WHERE 1=1"
	args := []any{}
	if u := current(r); !staff(u) {
		where += " AND o.user_id=?"
		args = append(args, u.ID)
	}
	if s := r.URL.Query().Get("status"); s != "" {
		if !validStatus(s) {
			return bad("Неизвестный статус")
		}
		where += " AND o.status=?"
		args = append(args, s)
	}
	var from, to time.Time
	for _, key := range []string{"from", "to"} {
		if s := r.URL.Query().Get(key); s != "" {
			t, e := time.Parse(time.RFC3339, s)
			if e != nil {
				return bad(key + ": ожидается RFC3339, например 2026-11-16T00:00:00Z")
			}
			op := ">="
			if key == "to" {
				op = "<="
				to = t
			} else {
				from = t
			}
			where += " AND o.date_time" + op + "?"
			args = append(args, t.UTC().Format(time.RFC3339))
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return bad("Начало интервала позже конца")
	}
	var total int
	if err = a.db.QueryRow("SELECT count(*) FROM orders o"+where, args...).Scan(&total); err != nil {
		return err
	}
	items, err := rowsJSON(a.db, orderSelect+where+" ORDER BY o.date_time DESC,o.rowid DESC LIMIT ? OFFSET ?", append(args, l, (p-1)*l)...)
	if err != nil {
		return err
	}
	return respond(w, 200, map[string]any{"items": items, "total": total, "page": p, "limit": l})
}
func (a *App) authorizedOrder(r *http.Request, id string) (map[string]any, error) {
	o, err := a.one(orderSelect+" WHERE o.id=?", id)
	if err != nil {
		return nil, err
	}
	if !staff(current(r)) && o["user_id"] != current(r).ID {
		return nil, missing()
	}
	return o, nil
}
func (a *App) order(w http.ResponseWriter, r *http.Request) error {
	o, err := a.authorizedOrder(r, r.PathValue("id"))
	if err != nil {
		return err
	}
	items, err := a.items(r.PathValue("id"))
	if err != nil {
		return err
	}
	o["items"] = items
	if staff(current(r)) {
		customer, err := a.one("SELECT id,name,phone,email,registration_date FROM users WHERE id=?", o["user_id"])
		if err != nil {
			return err
		}
		o["customer"] = customer
	}
	return respond(w, 200, o)
}
func (a *App) items(id string) ([]map[string]any, error) {
	return rowsJSON(a.db, `SELECT i.id,i.order_id,i.product_id,i.price,i.count,p.name,p.size,p.color,p.image_index,(SELECT id FROM photos WHERE product_id=p.id ORDER BY rowid LIMIT 1) AS photo_id FROM order_items i JOIN products p ON p.id=i.product_id WHERE i.order_id=? ORDER BY i.rowid`, id)
}
func (a *App) orderItems(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.authorizedOrder(r, r.PathValue("order_id")); err != nil {
		return err
	}
	items, err := a.items(r.PathValue("order_id"))
	if err != nil {
		return err
	}
	return respond(w, 200, items)
}
func (a *App) updateOrder(w http.ResponseWriter, r *http.Request) error {
	var v struct {
		Status             string `json:"status"`
		CancellationReason string `json:"cancellation_reason"`
	}
	if err := decode(w, r, &v); err != nil {
		return err
	}
	if !validStatus(v.Status) {
		return bad("Неизвестный статус")
	}
	u := current(r)
	if !staff(u) && v.Status != "Отменён" {
		return deny()
	}
	v.CancellationReason = strings.TrimSpace(v.CancellationReason)
	if utf8.RuneCountInString(v.CancellationReason) > 1000 {
		return bad("Причина отмены: не более 1000 символов")
	}
	if v.Status != "Отменён" && v.CancellationReason != "" {
		return bad("Причина указывается только при отмене заказа")
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old, owner, reason string
	if err = tx.QueryRow("SELECT status,user_id,cancellation_reason FROM orders WHERE id=?", r.PathValue("id")).Scan(&old, &owner, &reason); err != nil {
		return err
	}
	if !staff(u) && owner != u.ID {
		return missing()
	}
	if old == "Отменён" && v.Status != old {
		return bad("Недопустимый переход статуса заказа")
	}
	if !staff(u) && old != "Принят" && old != "В работе" {
		return bad("Покупатель может отменить заказ только в статусах «Принят» и «В работе»")
	}
	if old == "Отменён" {
		if v.CancellationReason != "" && v.CancellationReason != reason {
			return bad("Причину уже отменённого заказа менять нельзя")
		}
	} else if v.Status == "Отменён" {
		if v.CancellationReason == "" {
			if staff(u) {
				return bad("Укажите причину отмены заказа")
			}
			v.CancellationReason = "Отменён покупателем"
		}
		if _, err = tx.Exec(`UPDATE products SET stock=stock+COALESCE((SELECT SUM(reserved_count) FROM order_items WHERE order_id=? AND product_id=products.id),0) WHERE id IN (SELECT product_id FROM order_items WHERE order_id=?)`, r.PathValue("id"), r.PathValue("id")); err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE order_items SET reserved_count=0 WHERE order_id=?", r.PathValue("id")); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE orders SET status=?,cancellation_reason=?,cancellation_actor=?,cancelled_by=?,cancelled_at=? WHERE id=?", v.Status, v.CancellationReason, u.Role, u.ID, time.Now().UTC().Format(time.RFC3339), r.PathValue("id"))
	} else {
		_, err = tx.Exec("UPDATE orders SET status=? WHERE id=?", v.Status, r.PathValue("id"))
	}
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	o, err := a.one(orderSelect+" WHERE o.id=?", r.PathValue("id"))
	if err != nil {
		return err
	}
	return respond(w, 200, o)
}
func (a *App) addOrderItem(w http.ResponseWriter, r *http.Request) error {
	var v struct {
		OrderID   string `json:"order_id"`
		ProductID string `json:"product_id"`
		Price     *int64 `json:"price"`
		Count     int    `json:"count"`
	}
	if err := decode(w, r, &v); err != nil {
		return err
	}
	if v.Count == 0 {
		v.Count = 1
	}
	if v.Count < 1 || v.Count > 99 {
		return bad("Количество: от 1 до 99")
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRow("SELECT status FROM orders WHERE id=?", v.OrderID).Scan(&status); err != nil {
		return err
	}
	if status != "Принят" {
		return bad("Состав можно менять только у принятого заказа")
	}
	var price int64
	if err = tx.QueryRow("SELECT price FROM products WHERE id=? AND deleted=0", v.ProductID).Scan(&price); err != nil {
		return err
	}
	if v.Price != nil && *v.Price != price {
		return bad("Цена должна совпадать с ценой каталога")
	}
	if err = reserveStock(tx, v.ProductID, v.Count); err != nil {
		return err
	}
	id := store.ID()
	if _, err = tx.Exec("INSERT INTO order_items(id,order_id,product_id,price,count,reserved_count) VALUES(?,?,?,?,?,?)", id, v.OrderID, v.ProductID, price, v.Count, v.Count); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return respond(w, 201, map[string]any{"id": id, "order_id": v.OrderID, "product_id": v.ProductID, "price": price, "count": v.Count})
}

func (a *App) addCartBatch(w http.ResponseWriter, r *http.Request) error {
	var v struct {
		Items []cartInput `json:"items"`
	}
	if err := decode(w, r, &v); err != nil {
		return err
	}
	if len(v.Items) == 0 || len(v.Items) > 25 {
		return bad("Выберите от 1 до 25 вариантов товара")
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	seen := map[string]bool{}
	for _, item := range v.Items {
		if item.Count < 1 || item.Count > 99 || item.ProductID == "" || seen[item.ProductID] || (item.UserID != "" && item.UserID != current(r).ID) {
			return bad("Проверьте размеры и количество")
		}
		seen[item.ProductID] = true
		result, e := tx.Exec(`INSERT INTO carts(id,user_id,product_id,count) SELECT ?,?,?,? WHERE (SELECT stock FROM products WHERE id=? AND deleted=0)>=?+COALESCE((SELECT count FROM carts WHERE user_id=? AND product_id=?),0) ON CONFLICT(user_id,product_id) DO UPDATE SET count=count+excluded.count`, store.ID(), current(r).ID, item.ProductID, item.Count, item.ProductID, item.Count, current(r).ID, item.ProductID)
		if e != nil {
			return e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if n == 0 {
			return bad("Недостаточно товара выбранного размера на складе. Обновите карточку.")
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return respond(w, 201, map[string]bool{"ok": true})
}
