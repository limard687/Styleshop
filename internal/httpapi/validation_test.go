package httpapi

import (
	"net/url"
	"strings"
	"testing"
)

func TestUserContactValidation(t *testing.T) {
	for _, email := range []string{"a@b", "a@b.c", "a@.com", "a@b..com", ".a@b.com", "a.@b.com", "a..b@c.com", "a@-b.com", "a@b-.com", "a@b.c0m", "a b@c.com", "a@b.com\n", "a@@b.com", "имя@example.com", strings.Repeat("a", 65) + "@b.com", "a@" + strings.Repeat("a", 64) + ".com", strings.Repeat("x", 255)} {
		if validEmail(email) {
			t.Errorf("accepted invalid email %q", email)
		}
	}
	for _, email := range []string{"local@domain.tld", "first.last+tag@mail.example.ru", "a_b-c@example.com", "a@example-domain.co.uk", "client@styleshop.local"} {
		if !validEmail(email) {
			t.Errorf("rejected valid email %q", email)
		}
	}
	for input, want := range map[string]string{"+79001234567": "+79001234567", "89001234567": "+79001234567", "+7 (900) 123-45-67": "+79001234567", "8 (495) 123-45-67": "+74951234567", "+7 812 123-45-67": "+78121234567"} {
		got, err := normalizePhone(input)
		if err != nil || got != want {
			t.Errorf("phone %q: got %q, %v", input, got, err)
		}
	}
	for _, phone := range []string{"", "9001234567", "+12025550123", "+375291234567", "+77001234567", "+76001234567", "+7900123456", "+790012345678", "++79001234567", "+7call9001234567", "+7９００1234567", strings.Repeat("1", 1000)} {
		if _, err := normalizePhone(phone); err == nil {
			t.Errorf("accepted invalid phone %q", phone)
		}
	}
	for _, field := range []string{"name", "email", "phone", "password"} {
		v := userInput{Name: "Имя", Email: "a@example.com", Phone: "+79001234567", Password: "Password2026!"}
		switch field {
		case "name":
			v.Name = strings.Repeat("я", 101)
		case "email":
			v.Email = strings.Repeat("a", 255)
		case "phone":
			v.Phone = strings.Repeat("1", 19)
		case "password":
			v.Password = strings.Repeat("я", 37)
		}
		if err := validUser(&v); err == nil {
			t.Errorf("accepted oversized %s", field)
		}
	}
}

func TestValidationCannotBeBypassedThroughAPI(t *testing.T) {
	f := newFixture(t)
	admin := f.login("admin")
	user := map[string]any{"name": "Имя", "email": "a@localhost", "phone": "+79001234567", "password": "Password2026!", "password_confirmation": "Password2026!", "privacy_accepted": true}
	f.request("POST", "/user", "", user, 400)
	f.request("PUT", "/user/10000000-0000-4000-8000-000000000001", admin, user, 400)
	user["email"], user["phone"] = "a@example.com", "+77001234567"
	f.request("POST", "/user", "", user, 400)
	f.request("PUT", "/user/10000000-0000-4000-8000-000000000001", admin, user, 400)
	f.request("POST", "/auth/login", "", map[string]string{"email": "admin@form", "password": "StyleDemo2026!"}, 400)
	f.request("POST", "/auth/login", "", map[string]string{"email": "admin@styleshop.local", "password": strings.Repeat("p", 73)}, 400)
	f.request("POST", "/role", admin, map[string]string{"name": strings.Repeat("я", 51)}, 400)
	product := map[string]any{"name": "Товар", "type": "Футболки", "gender": "Унисекс", "size": "M", "color": "Серый", "description": "Описание", "brand": "StyleShop", "season": "Лето", "price": 10000}
	for _, field := range []string{"name", "type", "size", "color", "brand", "description"} {
		old := product[field]
		product[field] = strings.Repeat("я", map[string]int{"name": 121, "type": 121, "size": 21, "color": 61, "brand": 121, "description": 4001}[field])
		f.request("POST", "/product", admin, product, 400)
		product[field] = old
	}
	f.request("GET", "/product?"+url.Values{"q": {strings.Repeat("я", 101)}}.Encode(), "", nil, 400)
	f.request("GET", "/product?q="+strings.Repeat("x", 9000), "", nil, 414)
	user["name"] = strings.Repeat("x", 70000)
	f.request("POST", "/user", "", user, 400)
}

func TestPasswordConfirmationAndCharacterLength(t *testing.T) {
	f := newFixture(t)
	user := map[string]any{"name": "Покупатель", "email": "password@example.com", "phone": "+79001234567", "privacy_accepted": true}
	for _, password := range []string{"1234567", "паролья", strings.Repeat("я", 37)} {
		user["password"], user["password_confirmation"] = password, password
		out := obj(f.request("POST", "/user", "", user, 400))
		if strings.Contains(strings.ToLower(out["error"].(string)), "байт") {
			t.Fatal("technical password limit leaked to the user")
		}
	}
	user["password"] = "пароль12"
	delete(user, "password_confirmation")
	f.request("POST", "/user", "", user, 400)
	user["password_confirmation"] = "другой12"
	f.request("POST", "/user", "", user, 400)
	user["password_confirmation"] = user["password"]
	created := obj(f.request("POST", "/user", "", user, 201))
	token := obj(f.request("POST", "/auth/login", "", map[string]string{"email": "password@example.com", "password": "пароль12"}, 200))["token"].(string)
	f.request("PATCH", "/auth/me", token, map[string]string{"password": "NewPass8"}, 400)
	f.request("PATCH", "/auth/me", token, map[string]string{"password": "NewPass8", "password_confirmation": "NewPass9"}, 400)
	f.request("PATCH", "/auth/me", token, map[string]string{"password": "NewPass8", "password_confirmation": "NewPass8"}, 200)
	f.request("POST", "/auth/login", "", map[string]string{"email": "password@example.com", "password": "NewPass8"}, 200)

	admin := f.login("admin")
	user["password"], user["password_confirmation"] = "Admin123", "Mismatch"
	f.request("PUT", "/user/"+created["id"].(string), admin, user, 400)
	user["password_confirmation"] = user["password"]
	f.request("PUT", "/user/"+created["id"].(string), admin, user, 200)
}
