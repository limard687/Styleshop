package httpapi

import (
	"clothing-store/internal/store"
	"database/sql"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const userSelect = `SELECT u.id,u.name,u.email,u.phone,u.role_id,r.name AS role,u.registration_date FROM users u JOIN roles r ON r.id=u.role_id`

var emailLocal = regexp.MustCompile("^[a-z0-9.!#$%&'*+/=?^_`{|}~-]+$")
var domainLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
var domainZone = regexp.MustCompile(`^[a-z]{2,63}$`)
var russianPhone = regexp.MustCompile(`^\+7[3489][0-9]{9}$`)

func validEmail(email string) bool {
	if len(email) > 254 || strings.Count(email, "@") != 1 {
		return false
	}
	local, domain, _ := strings.Cut(email, "@")
	if len(local) == 0 || len(local) > 64 || !emailLocal.MatchString(local) || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") || len(domain) > 253 {
		return false
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 || !domainZone.MatchString(labels[len(labels)-1]) {
		return false
	}
	for _, label := range labels {
		if !domainLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func normalizePhone(phone string) (string, error) {
	if len(phone) > 18 {
		return "", bad("Телефон: российский номер +7 XXX XXX-XX-XX или 8XXXXXXXXXX")
	}
	var compact strings.Builder
	for _, char := range phone {
		switch {
		case char >= '0' && char <= '9', char == '+':
			compact.WriteRune(char)
		case char == ' ', char == '(', char == ')', char == '-':
		default:
			return "", bad("В телефоне допустимы только цифры, +, пробелы, скобки и дефисы")
		}
	}
	value := compact.String()
	if len(value) == 11 && value[0] == '8' {
		value = "+7" + value[1:]
	}
	if !russianPhone.MatchString(value) {
		return "", bad("Укажите российский номер: +7 XXX XXX-XX-XX или 8XXXXXXXXXX")
	}
	return value, nil
}

type userInput struct {
	Name                 string `json:"name"`
	Email                string `json:"email"`
	Phone                string `json:"phone"`
	RoleID               string `json:"role_id"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"password_confirmation"`
	PrivacyAccepted      bool   `json:"privacy_accepted"`
}

func validUser(v *userInput) error {
	v.Name = strings.TrimSpace(v.Name)
	v.Email = strings.ToLower(strings.TrimSpace(v.Email))
	v.Phone = strings.TrimSpace(v.Phone)
	if len(v.Name) == 0 || utf8.RuneCountInString(v.Name) > 100 {
		return bad("Имя: от 1 до 100 символов")
	}
	if !validEmail(v.Email) {
		return bad("Почта должна быть в формате local@domain.tld")
	}
	phone, err := normalizePhone(v.Phone)
	if err != nil {
		return err
	}
	v.Phone = phone
	if v.Password != "" {
		if err := validPassword(v.Password); err != nil {
			return err
		}
		if v.Password != v.PasswordConfirmation {
			return bad("Пароли не совпадают")
		}
	}
	return nil
}
func validPassword(password string) error {
	if utf8.RuneCountInString(password) < 8 {
		return bad("Пароль должен содержать минимум 8 символов")
	}
	if len(password) > 72 {
		return bad("Пароль слишком длинный. Используйте более короткий пароль")
	}
	return nil
}

func (a *App) login(w http.ResponseWriter, r *http.Request) error {
	if err := a.throttle(r); err != nil {
		return err
	}
	var v struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &v); err != nil {
		return err
	}
	v.Email = strings.ToLower(strings.TrimSpace(v.Email))
	if !validEmail(v.Email) {
		return bad("Почта должна быть в формате local@domain.tld")
	}
	if err := validPassword(v.Password); err != nil {
		return err
	}
	var id, hash string
	err := a.db.QueryRow("SELECT id,password_hash FROM users WHERE email=?", strings.ToLower(strings.TrimSpace(v.Email))).Scan(&id, &hash)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == sql.ErrNoRows {
		hash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(v.Password)) != nil || id == "" {
		return apiError{401, "Неверный email или пароль"}
	}
	token, err := a.newSession(w, id)
	if err != nil {
		return err
	}
	u, err := a.one(userSelect+" WHERE u.id=?", id)
	if err != nil {
		return err
	}
	return respond(w, 200, map[string]any{"user": u, "token": token})
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) error {
	token := ""
	if c, err := r.Cookie("styleshop_session"); err == nil {
		token = c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token = strings.TrimPrefix(h, "Bearer ")
	}
	if _, err := a.db.Exec("DELETE FROM sessions WHERE token_hash=?", a.hashToken(token)); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "styleshop_session", Value: "", Path: "/", HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	return respond(w, 200, map[string]bool{"ok": true})
}
func (a *App) createUser(w http.ResponseWriter, r *http.Request) error {
	if err := a.throttle(r); err != nil {
		return err
	}
	var v userInput
	if err := decode(w, r, &v); err != nil {
		return err
	}
	if err := validUser(&v); err != nil {
		return err
	}
	if v.Password == "" {
		return bad("Укажите пароль")
	}
	if v.Phone == "" {
		return bad("Укажите телефон")
	}
	publicRegistration := current(r) == nil || current(r).Role != "admin"
	if publicRegistration && !v.PrivacyAccepted {
		return bad("Подтвердите согласие с политикой конфиденциальности")
	}
	if u := current(r); u == nil || u.Role != "admin" {
		if v.RoleID != "" && v.RoleID != "00000000-0000-4000-8000-000000000001" {
			return deny()
		}
		v.RoleID = "00000000-0000-4000-8000-000000000001"
	} else if v.RoleID == "" {
		v.RoleID = "00000000-0000-4000-8000-000000000001"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(v.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	id := store.ID()
	acceptedAt, policyVersion := "", ""
	if publicRegistration {
		acceptedAt = time.Now().UTC().Format(time.RFC3339)
		policyVersion = "2026-11-16.2"
	}
	_, err = a.db.Exec(`INSERT INTO users(id,name,email,phone,role_id,registration_date,password_hash,privacy_accepted_at,privacy_version) VALUES(?,?,?,?,?,?,?,?,?)`, id, v.Name, v.Email, v.Phone, v.RoleID, time.Now().UTC().Format(time.RFC3339), string(hash), acceptedAt, policyVersion)
	if err != nil {
		return err
	}
	u, err := a.one(userSelect+" WHERE u.id=?", id)
	if err != nil {
		return err
	}
	return respond(w, 201, u)
}
func (a *App) users(w http.ResponseWriter, r *http.Request) error {
	p, l, err := pagination(r)
	if err != nil {
		return err
	}
	where := ""
	args := []any{}
	if id := r.PathValue("id"); id != "" {
		where = " WHERE u.role_id=?"
		args = append(args, id)
	}
	var total int
	if err = a.db.QueryRow("SELECT count(*) FROM users u"+where, args...).Scan(&total); err != nil {
		return err
	}
	items, err := rowsJSON(a.db, userSelect+where+" ORDER BY u.registration_date,u.id LIMIT ? OFFSET ?", append(args, l, (p-1)*l)...)
	if err != nil {
		return err
	}
	return respond(w, 200, map[string]any{"items": items, "total": total, "page": p, "limit": l})
}
func (a *App) updateUser(w http.ResponseWriter, r *http.Request) error {
	u := current(r)
	id := r.PathValue("id")
	if u.Role != "admin" && u.ID != id {
		return deny()
	}
	var v userInput
	if err := decode(w, r, &v); err != nil {
		return err
	}
	if err := validUser(&v); err != nil {
		return err
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldRole, hash string
	if err = tx.QueryRow("SELECT role_id,password_hash FROM users WHERE id=?", id).Scan(&oldRole, &hash); err != nil {
		return err
	}
	if v.RoleID == "" {
		v.RoleID = oldRole
	}
	if u.Role != "admin" && v.RoleID != oldRole {
		return deny()
	}
	if oldRole == "00000000-0000-4000-8000-000000000003" && v.RoleID != oldRole {
		var n int
		if err = tx.QueryRow("SELECT count(*) FROM users WHERE role_id=?", oldRole).Scan(&n); err != nil {
			return err
		}
		if n <= 1 {
			return bad("Нельзя понизить последнего администратора")
		}
	}
	if v.Password != "" {
		b, e := bcrypt.GenerateFromPassword([]byte(v.Password), bcrypt.DefaultCost)
		if e != nil {
			return e
		}
		hash = string(b)
	}
	if _, err = tx.Exec("UPDATE users SET name=?,email=?,phone=?,role_id=?,password_hash=? WHERE id=?", v.Name, v.Email, v.Phone, v.RoleID, hash, id); err != nil {
		return err
	}
	if v.Password != "" || v.RoleID != oldRole {
		if _, err = tx.Exec("DELETE FROM sessions WHERE user_id=?", id); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	out, err := a.one(userSelect+" WHERE u.id=?", id)
	if err != nil {
		return err
	}
	return respond(w, 200, out)
}

func (a *App) updateProfile(w http.ResponseWriter, r *http.Request) error {
	var v struct {
		Name                 *string `json:"name"`
		Email                *string `json:"email"`
		Phone                *string `json:"phone"`
		Password             *string `json:"password"`
		PasswordConfirmation *string `json:"password_confirmation"`
	}
	if err := decode(w, r, &v); err != nil {
		return err
	}
	count := 0
	for _, value := range []*string{v.Name, v.Email, v.Phone, v.Password} {
		if value != nil {
			count++
		}
	}
	if count != 1 || (v.PasswordConfirmation != nil && v.Password == nil) {
		return bad("Изменяйте одно поле за раз")
	}
	column, value := "", ""
	switch {
	case v.Name != nil:
		column, value = "name", strings.TrimSpace(*v.Name)
		if value == "" || utf8.RuneCountInString(value) > 100 {
			return bad("Имя: от 1 до 100 символов")
		}
	case v.Email != nil:
		column, value = "email", strings.ToLower(strings.TrimSpace(*v.Email))
		if !validEmail(value) {
			return bad("Почта должна быть в формате local@domain.tld")
		}
	case v.Phone != nil:
		var err error
		column = "phone"
		value, err = normalizePhone(strings.TrimSpace(*v.Phone))
		if err != nil {
			return err
		}
	case v.Password != nil:
		if err := validPassword(*v.Password); err != nil {
			return err
		}
		if v.PasswordConfirmation == nil || *v.PasswordConfirmation != *v.Password {
			return bad("Пароли не совпадают")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*v.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		column, value = "password_hash", string(hash)
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = affected(tx.Exec("UPDATE users SET "+column+"=? WHERE id=?", value, current(r).ID)); err != nil {
		return err
	}
	if v.Password != nil {
		if _, err = tx.Exec("DELETE FROM sessions WHERE user_id=?", current(r).ID); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if v.Password != nil {
		http.SetCookie(w, &http.Cookie{Name: "styleshop_session", Value: "", Path: "/", HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	}
	out, err := a.one(userSelect+" WHERE u.id=?", current(r).ID)
	if err != nil {
		return err
	}
	return respond(w, 200, out)
}

func (a *App) deleteUser(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if id == current(r).ID {
		return bad("Нельзя удалить свой аккаунт администратора")
	}
	if err := affected(a.db.Exec("DELETE FROM users WHERE id=?", id)); err != nil {
		return err
	}
	return respond(w, 200, map[string]bool{"ok": true})
}
func (a *App) roles(w http.ResponseWriter, r *http.Request) error {
	items, err := rowsJSON(a.db, "SELECT id,name FROM roles ORDER BY name")
	if err != nil {
		return err
	}
	return respond(w, 200, items)
}
func (a *App) saveRole(w http.ResponseWriter, r *http.Request) error {
	var v struct {
		Name string `json:"name"`
	}
	if err := decode(w, r, &v); err != nil {
		return err
	}
	v.Name = strings.TrimSpace(v.Name)
	if len(v.Name) == 0 || utf8.RuneCountInString(v.Name) > 50 {
		return bad("Название роли: 1–50 символов")
	}
	id := r.PathValue("id")
	code := 200
	if id == "" {
		id = store.ID()
		code = 201
		if _, err := a.db.Exec("INSERT INTO roles(id,name) VALUES(?,?)", id, v.Name); err != nil {
			return err
		}
	} else {
		if builtinRole(id) {
			return bad("Системные роли нельзя переименовывать")
		}
		if v.Name == "admin" || v.Name == "manager" || v.Name == "client" {
			return bad("Зарезервированное название роли")
		}
		if err := affected(a.db.Exec("UPDATE roles SET name=? WHERE id=?", v.Name, id)); err != nil {
			return err
		}
	}
	return respond(w, code, map[string]string{"id": id, "name": v.Name})
}
func builtinRole(id string) bool {
	return id == "00000000-0000-4000-8000-000000000001" || id == "00000000-0000-4000-8000-000000000002" || id == "00000000-0000-4000-8000-000000000003"
}
func (a *App) deleteRole(w http.ResponseWriter, r *http.Request) error {
	if builtinRole(r.PathValue("id")) {
		return bad("Системные роли нельзя удалять")
	}
	if err := affected(a.db.Exec("DELETE FROM roles WHERE id=?", r.PathValue("id"))); err != nil {
		return err
	}
	return respond(w, 200, map[string]bool{"ok": true})
}
