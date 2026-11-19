package httpapi

import "testing"

func TestOrderCustomerContactsAreOnlyAvailableToStaff(t *testing.T) {
	f := newFixture(t)
	client, admin, manager := f.login("client"), f.login("admin"), f.login("manager")
	f.request("PATCH", "/auth/me", client, map[string]string{"phone": "+79001234567"}, 200)
	product := obj(list(obj(f.request("GET", "/product", "", nil, 200))["items"])[0])
	f.request("POST", "/cart", client, map[string]any{"product_id": product["id"], "count": 1}, 201)
	order := obj(f.request("POST", "/order", client, map[string]any{}, 201))
	path := "/order/" + order["id"].(string)
	for _, token := range []string{admin, manager} {
		detail := obj(f.request("GET", path, token, nil, 200))
		customer := obj(detail["customer"])
		if customer["name"] != "Покупатель" || customer["email"] != "client@styleshop.local" || customer["phone"] != "+79001234567" || customer["id"] != order["user_id"] || customer["registration_date"] == "" {
			t.Fatal(customer)
		}
		if len(customer) != 5 {
			t.Fatal("unexpected customer fields", customer)
		}
		for _, key := range []string{"id", "name", "email", "phone", "registration_date"} {
			if _, ok := customer[key]; !ok {
				t.Fatal("missing field", key)
			}
		}
	}
	if detail := obj(f.request("GET", path, client, nil, 200)); detail["customer"] != nil {
		t.Fatal("customer block was returned to buyer", detail)
	}
	f.request("GET", path, "", nil, 401)
	f.request("POST", "/user", "", map[string]any{"name": "Другой покупатель", "email": "other@example.com", "phone": "+79000000000", "password": "Password123!", "password_confirmation": "Password123!", "privacy_accepted": true}, 201)
	other := obj(f.request("POST", "/auth/login", "", map[string]string{"email": "other@example.com", "password": "Password123!"}, 200))["token"].(string)
	f.request("GET", path, other, nil, 404)
}
