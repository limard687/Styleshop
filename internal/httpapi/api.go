package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"clothing-store/internal/config"
	"clothing-store/web"

	"github.com/mattn/go-sqlite3"
)

type App struct {
	db       *sql.DB
	cfg      config.Config
	log      *slog.Logger
	mux      *http.ServeMux
	mu       sync.Mutex
	attempts map[string]attempt
}
type attempt struct {
	count int
	until time.Time
}
type User struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Phone            string `json:"phone"`
	RoleID           string `json:"role_id"`
	Role             string `json:"role"`
	RegistrationDate string `json:"registration_date"`
}
type userKey struct{}
type apiError struct {
	code    int
	message string
}

func (e apiError) Error() string { return e.message }
func bad(s string) error         { return apiError{400, s} }
func deny() error                { return apiError{403, "Недостаточно прав"} }
func missing() error             { return apiError{404, "Запись не найдена"} }

type endpoint func(http.ResponseWriter, *http.Request) error

func New(db *sql.DB, c config.Config, log *slog.Logger) (*App, error) {
	if err := os.MkdirAll(c.UploadDir, 0700); err != nil {
		return nil, err
	}
	a := &App{db: db, cfg: c, log: log, mux: http.NewServeMux(), attempts: map[string]attempt{}}
	if c.SeedDemo {
		if err := a.seedStyleShopPhotos(); err != nil {
			return nil, err
		}
	}
	a.routes()
	static, err := fs.Sub(web.Files, "static")
	if err != nil {
		return nil, err
	}
	a.mux.Handle("GET /", http.FileServer(http.FS(static)))
	return a, nil
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	sw := &statusWriter{w, 200}
	defer func() {
		if recovered := recover(); recovered != nil {
			a.log.Error("request panic", "error", fmt.Sprint(recovered))
			fail(sw, 500, "Внутренняя ошибка сервера")
		}
		a.log.Info("http request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "duration_ms", time.Since(start).Milliseconds())
	}()
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if len(r.RequestURI) > 8192 {
		fail(w, 414, "Адрес запроса слишком длинный")
		return
	}
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
	if !strings.HasPrefix(r.URL.Path, "/assets/") {
		w.Header().Set("Cache-Control", "no-store")
	}
	if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			fail(sw, 403, "Недопустимый источник запроса")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
				fail(sw, 403, "Недопустимый источник запроса")
				return
			}
		}
	}
	u, err := a.authenticate(r)
	if err != nil {
		a.log.Error("session lookup failed", "error", err)
		fail(sw, 500, "Ошибка проверки сессии")
		return
	}
	if u != nil {
		r = r.WithContext(context.WithValue(r.Context(), userKey{}, u))
	}
	a.mux.ServeHTTP(sw, r)
}
func current(r *http.Request) *User { u, _ := r.Context().Value(userKey{}).(*User); return u }
func staff(u *User) bool            { return u != nil && (u.Role == "admin" || u.Role == "manager") }
func (a *App) handle(pattern string, roles string, h endpoint) {
	a.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if roles != "" {
			u := current(r)
			if u == nil {
				fail(w, 401, "Сначала войдите в аккаунт")
				return
			}
			if roles != "*" && !strings.Contains(","+roles+",", ","+u.Role+",") {
				message := "Недостаточно прав"
				if strings.HasPrefix(r.URL.Path, "/cart") {
					message = "Корзина доступна только покупателям"
				}
				fail(w, 403, message)
				return
			}
		}
		if err := h(w, r); err != nil {
			var e apiError
			var se sqlite3.Error
			switch {
			case errors.As(err, &e):
				fail(w, e.code, e.message)
			case errors.Is(err, sql.ErrNoRows):
				fail(w, 404, "Запись не найдена")
			case errors.As(err, &se) && se.Code == sqlite3.ErrConstraint:
				fail(w, 409, "Конфликт данных: запись уже существует или используется")
			default:
				a.log.Error("handler failed", "path", r.URL.Path, "error", err)
				fail(w, 500, "Внутренняя ошибка сервера")
			}
		}
	})
}
func respond(w http.ResponseWriter, code int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	return json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, msg string) {
	_ = respond(w, code, map[string]any{"error": msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if ct := strings.Split(r.Header.Get("Content-Type"), ";")[0]; ct != "application/json" {
		return apiError{415, "Ожидается Content-Type: application/json"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return bad("Некорректный JSON или неизвестное поле")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return bad("Ожидается один JSON-объект")
	}
	return nil
}
func pagination(r *http.Request) (int, int, error) {
	p, l := 1, 12
	var err error
	if s := r.URL.Query().Get("page"); s != "" {
		p, err = strconv.Atoi(s)
		if err != nil || p < 1 || p > 1000000 {
			return 0, 0, bad("page: число от 1 до 1000000")
		}
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		l, err = strconv.Atoi(s)
		if err != nil || l < 1 || l > 100 {
			return 0, 0, bad("limit: число от 1 до 100")
		}
	}
	return p, l, nil
}

type querier interface {
	Query(string, ...any) (*sql.Rows, error)
}

func rowsJSON(db querier, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err = rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := map[string]any{}
		for i, col := range cols {
			if b, ok := values[i].([]byte); ok {
				m[col] = string(b)
			} else {
				m[col] = values[i]
			}
		}
		if raw, ok := m["attributes"].(string); ok {
			var attrs map[string][]string
			if err := json.Unmarshal([]byte(raw), &attrs); err != nil {
				return nil, err
			}
			m["attributes"] = attrs
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (a *App) one(query string, args ...any) (map[string]any, error) {
	rows, err := rowsJSON(a.db, query, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	return rows[0], nil
}
func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return missing()
	}
	return nil
}
func (a *App) hashToken(token string) string {
	h := hmac.New(sha256.New, []byte(a.cfg.SessionSecret))
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}
func (a *App) authenticate(r *http.Request) (*User, error) {
	token := ""
	if c, e := r.Cookie("styleshop_session"); e == nil {
		token = c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token = strings.TrimPrefix(h, "Bearer ")
	}
	if token == "" {
		return nil, nil
	}
	u := &User{}
	err := a.db.QueryRow(`SELECT u.id,u.name,u.email,u.phone,u.role_id,r.name,u.registration_date FROM sessions s JOIN users u ON u.id=s.user_id JOIN roles r ON r.id=u.role_id WHERE s.token_hash=? AND s.expires_at>?`, a.hashToken(token), time.Now().UTC().Format(time.RFC3339)).Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.RoleID, &u.Role, &u.RegistrationDate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}
func (a *App) newSession(w http.ResponseWriter, userID string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	if _, err := a.db.Exec("DELETE FROM sessions WHERE expires_at<=?", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return "", err
	}
	if _, err := a.db.Exec("INSERT INTO sessions(token_hash,user_id,expires_at) VALUES(?,?,?)", a.hashToken(token), userID, time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339)); err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{Name: "styleshop_session", Value: token, Path: "/", HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 86400})
	return token, nil
}
func (a *App) throttle(r *http.Request) error {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	key := ip + ":" + r.URL.Path
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, v := range a.attempts {
		if now.After(v.until) {
			delete(a.attempts, k)
		}
	}
	v := a.attempts[key]
	if v.count == 0 {
		v.until = now.Add(time.Minute)
	}
	v.count++
	a.attempts[key] = v
	if v.count > 15 {
		return apiError{429, "Слишком много попыток. Повторите через минуту"}
	}
	return nil
}

func (a *App) routes() {
	a.handle("GET /health", "", func(w http.ResponseWriter, r *http.Request) error {
		if err := a.db.PingContext(r.Context()); err != nil {
			return err
		}
		return respond(w, 200, map[string]string{"status": "ok"})
	})
	a.handle("POST /auth/login", "", a.login)
	a.handle("POST /auth/logout", "*", a.logout)
	a.handle("GET /auth/me", "*", func(w http.ResponseWriter, r *http.Request) error { return respond(w, 200, current(r)) })
	a.handle("PATCH /auth/me", "*", a.updateProfile)
	a.handle("POST /product", "admin", a.saveProduct)
	a.handle("PATCH /product/{id}/stock", "admin", a.updateStock)
	a.handle("GET /product", "", a.products)
	a.handle("GET /product/options", "", a.productOptions)
	a.handle("GET /product/{id}", "", a.product)
	a.handle("PUT /product/{id}", "admin", a.saveProduct)
	a.handle("DELETE /product/{id}", "admin", a.deleteProduct)
	a.handle("POST /photo", "admin", a.uploadPhoto)
	a.handle("GET /photo/{id}", "", a.photo)
	a.handle("DELETE /photo/{id}", "admin", a.deletePhoto)
	a.handle("POST /role", "admin", a.saveRole)
	a.handle("GET /role", "admin", a.roles)
	a.handle("PUT /role/{id}", "admin", a.saveRole)
	a.handle("DELETE /role/{id}", "admin", a.deleteRole)
	a.handle("POST /user", "", a.createUser)
	a.handle("GET /user", "admin", a.users)
	a.handle("GET /user/role/{id}", "admin", a.users)
	a.handle("PUT /user/{id}", "*", a.updateUser)
	a.handle("DELETE /user/{id}", "admin", a.deleteUser)
	a.handle("POST /cart", "client", a.addCart)
	a.handle("POST /cart/batch", "client", a.addCartBatch)
	a.handle("GET /cart/{user_id}", "client", a.cart)
	a.handle("PUT /cart/{id}", "client", a.updateCart)
	a.handle("DELETE /cart/{id}", "client", a.deleteCart)
	a.handle("POST /order", "client", a.createOrder)
	a.handle("GET /order", "client,manager,admin", a.orders)
	a.handle("GET /order/{id}", "client,manager,admin", a.order)
	a.handle("PUT /order/{id}", "client,manager,admin", a.updateOrder)
	a.handle("POST /order_item", "admin", a.addOrderItem)
	a.handle("GET /order_item/{order_id}", "client,manager,admin", a.orderItems)
}
