package httpapi

import "testing"

func TestManagerCannotWriteCatalog(t *testing.T) {
	f := newFixture(t)
	manager := f.login("manager")
	p := obj(list(obj(f.request("GET", "/product", manager, nil, 200))["items"])[0])
	id := p["id"].(string)
	for _, request := range []struct{ method, path string }{
		{"POST", "/product"}, {"PUT", "/product/" + id}, {"DELETE", "/product/" + id}, {"PATCH", "/product/" + id + "/stock"}, {"POST", "/photo"}, {"DELETE", "/photo/any-id"},
	} {
		f.request(request.method, request.path, manager, map[string]any{}, 403)
	}
	f.request("GET", "/product/"+id, manager, nil, 200)
}
