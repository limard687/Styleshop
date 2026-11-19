package httpapi

import (
	"strings"
	"testing"
)

func TestIndependentProfileUpdates(t *testing.T) {
	f := newFixture(t)
	client := f.login("client")
	f.request("PATCH", "/auth/me", "", map[string]string{"name": "Имя"}, 401)
	u := obj(f.request("PATCH", "/auth/me", client, map[string]string{"name": "Новое имя"}, 200))
	if u["name"] != "Новое имя" || u["phone"] != "" || u["email"] != "client@styleshop.local" || u["role"] != "client" {
		t.Fatal(u)
	}
	u = obj(f.request("PATCH", "/auth/me", client, map[string]string{"phone": "8 (900) 123-45-67"}, 200))
	if u["phone"] != "+79001234567" || u["name"] != "Новое имя" || u["email"] != "client@styleshop.local" {
		t.Fatal(u)
	}
	u = obj(f.request("PATCH", "/auth/me", client, map[string]string{"email": " NEW@Example.com "}, 200))
	if u["email"] != "new@example.com" || u["name"] != "Новое имя" || u["phone"] != "+79001234567" {
		t.Fatal(u)
	}
	f.request("PATCH", "/auth/me", client, map[string]string{"email": "admin@styleshop.local"}, 409)
	for _, invalid := range []map[string]any{
		{}, {"name": nil}, {"name": "", "phone": "+79001234567"},
		{"role_id": "00000000-0000-4000-8000-000000000003"},
		{"id": "10000000-0000-4000-8000-000000000003", "name": "Чужое имя"},
		{"name": " "}, {"name": strings.Repeat("я", 101)},
		{"email": "invalid@local"}, {"email": strings.Repeat("a", 255)},
		{"phone": ""}, {"phone": "+77001234567"}, {"phone": strings.Repeat("1", 19)},
		{"password": ""}, {"password": "short"}, {"password": strings.Repeat("p", 73)},
	} {
		f.request("PATCH", "/auth/me", client, invalid, 400)
	}
	u = obj(f.request("GET", "/auth/me", client, nil, 200))
	if u["email"] != "new@example.com" || u["name"] != "Новое имя" || u["phone"] != "+79001234567" || u["role"] != "client" {
		t.Fatal(u)
	}
	for _, role := range []string{"admin", "manager"} {
		token := f.login(role)
		u = obj(f.request("PATCH", "/auth/me", token, map[string]string{"name": "Новое имя " + role}, 200))
		if u["role"] != role || u["email"] != role+"@styleshop.local" || u["name"] != "Новое имя "+role {
			t.Fatal(u)
		}
	}
}

func TestProfilePasswordChangeRevokesEverySession(t *testing.T) {
	f := newFixture(t)
	first, second := f.login("client"), f.login("client")
	u := obj(f.request("PATCH", "/auth/me", first, map[string]string{"password": "AnotherPassword2026!", "password_confirmation": "AnotherPassword2026!"}, 200))
	if _, exists := u["password_hash"]; exists || u["phone"] != "" || u["email"] != "client@styleshop.local" {
		t.Fatal(u)
	}
	f.request("GET", "/auth/me", first, nil, 401)
	f.request("GET", "/auth/me", second, nil, 401)
	f.request("POST", "/auth/login", "", map[string]string{"email": "client@styleshop.local", "password": "StyleDemo2026!"}, 401)
	f.request("POST", "/auth/login", "", map[string]string{"email": "client@styleshop.local", "password": "AnotherPassword2026!"}, 200)
}
