package httpapi

import (
	"clothing-store/internal/store"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const productSelect = `SELECT p.id,p.stock,p.name,p.type,p.gender,p.size,p.color,p.description,p.brand,p.season,p.price,p.image_index,p.attributes,(SELECT id FROM photos WHERE product_id=p.id ORDER BY rowid LIMIT 1) AS photo_id FROM products p`

type productInput struct {
	Stocks      map[string]map[string]int `json:"stock_by_variant"`
	Stock       *int                      `json:"stock"`
	Attributes  map[string][]string       `json:"attributes"`
	Name        string                    `json:"name"`
	Type        string                    `json:"type"`
	Gender      string                    `json:"gender"`
	Size        string                    `json:"size"`
	Color       string                    `json:"color"`
	Description string                    `json:"description"`
	Brand       string                    `json:"brand"`
	Season      string                    `json:"season"`
	Price       int64                     `json:"price"`
	ImageIndex  *int                      `json:"image_index"`
}

func (a *App) products(w http.ResponseWriter, r *http.Request) error {
	p, l, err := pagination(r)
	if err != nil {
		return err
	}
	where := " WHERE p.deleted=0"
	args := []any{}
	for _, key := range []string{"type", "gender", "size", "color", "season"} {
		values := r.URL.Query()[key]
		clauses := []string{}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if utf8.RuneCountInString(value) > 120 {
				return bad("Слишком длинное значение фильтра")
			}
			clauses = append(clauses, "EXISTS (SELECT 1 FROM json_each(COALESCE(json_extract(p.attributes,'$."+key+"'),json_array(p."+key+"))) WHERE casefold(value)=?)")
			args = append(args, strings.ToLower(value))
		}
		if len(clauses) > 20 {
			return bad("Слишком много значений фильтра")
		}
		if len(clauses) > 0 {
			where += " AND (" + strings.Join(clauses, " OR ") + ")"
		}
	}
	brandClauses := []string{}
	for _, brand := range r.URL.Query()["brand"] {
		brand = strings.TrimSpace(brand)
		if brand == "" {
			continue
		}
		if utf8.RuneCountInString(brand) > 120 {
			return bad("Название бренда слишком длинное")
		}
		brandClauses = append(brandClauses, "instr(casefold(p.brand),?)>0")
		args = append(args, strings.ToLower(brand))
	}
	if len(brandClauses) > 20 {
		return bad("Слишком много брендов")
	}
	if len(brandClauses) > 0 {
		where += " AND (" + strings.Join(brandClauses, " OR ") + ")"
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		if utf8.RuneCountInString(q) > 100 {
			return bad("Поисковая строка слишком длинная")
		}
		where += " AND (instr(casefold(p.name),?)>0 OR instr(casefold(p.description),?)>0 OR instr(casefold(p.brand),?)>0)"
		q = strings.ToLower(q)
		args = append(args, q, q, q)
	}
	order := "p.image_index,p.name,p.size,p.id"
	if s := r.URL.Query().Get("sort"); s != "" {
		switch s {
		case "price_asc":
			order = "p.price ASC,p.id"
		case "price_desc":
			order = "p.price DESC,p.id"
		default:
			return bad("sort: price_asc или price_desc")
		}
	}
	var total int
	if err = a.db.QueryRow("SELECT count(*) FROM products p"+where, args...).Scan(&total); err != nil {
		return err
	}
	items, err := rowsJSON(a.db, productSelect+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", append(args, l, (p-1)*l)...)
	if err != nil {
		return err
	}
	return respond(w, 200, map[string]any{"items": items, "total": total, "page": p, "limit": l})
}
func (a *App) product(w http.ResponseWriter, r *http.Request) error {
	p, err := a.one(productSelect+" WHERE p.id=? AND p.deleted=0", r.PathValue("id"))
	if err != nil {
		return err
	}
	photos, err := rowsJSON(a.db, "SELECT id,filename,product_id FROM photos WHERE product_id=? ORDER BY rowid", r.PathValue("id"))
	if err != nil {
		return err
	}
	p["photos"] = photos
	return respond(w, 200, p)
}
func (a *App) saveProduct(w http.ResponseWriter, r *http.Request) error {
	var v productInput
	files := []*multipart.FileHeader{}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			return bad("Неверная загрузка или запрос больше 32 МБ")
		}
		defer r.MultipartForm.RemoveAll()
		metadata := r.FormValue("product")
		if len(metadata) > 64000 {
			return bad("Данные товара слишком длинные")
		}
		d := json.NewDecoder(strings.NewReader(metadata))
		d.DisallowUnknownFields()
		if err := d.Decode(&v); err != nil {
			return bad("Поле product должно содержать JSON товара")
		}
		if err := d.Decode(new(any)); err != io.EOF {
			return bad("Ожидается один JSON-объект товара")
		}
		files = r.MultipartForm.File["file"]
		if len(files) > 10 {
			return bad("Можно загрузить до 10 фотографий за раз")
		}
	} else if err := decode(w, r, &v); err != nil {
		return err
	}
	if err := normalizeAttributes(&v); err != nil {
		return err
	}
	if len(strings.TrimSpace(v.Description)) == 0 || utf8.RuneCountInString(v.Description) > 4000 {
		return bad("Описание: 1-4000 символов")
	}
	if v.Price < 0 || v.Price > 100000000 {
		return bad("Цена: от 0 до 100000000 копеек")
	}
	if v.Stock != nil && (*v.Stock < 0 || *v.Stock > 1000000) {
		return bad("Остаток: от 0 до 1000000")
	}
	if v.Stocks != nil {
		for _, size := range v.Attributes["size"] {
			for _, color := range v.Attributes["color"] {
				qty, ok := v.Stocks[size][color]
				if !ok || qty < 0 || qty > 1000000 {
					return bad("Укажите остаток каждого размера и цвета от 0 до 1000000")
				}
			}
		}
	}
	saved := []uploadedPhoto{}
	committed := false
	defer func() {
		if !committed {
			for _, photo := range saved {
				os.Remove(filepath.Join(a.cfg.UploadDir, photo.Filename))
			}
		}
	}()
	for _, header := range files {
		photo, err := a.savePhotoFile(header)
		if err != nil {
			return err
		}
		saved = append(saved, photo)
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	id := r.PathValue("id")
	code := 200
	imageIndex := -1
	if id != "" {
		if err = tx.QueryRow("SELECT image_index FROM products WHERE id=? AND deleted=0", id).Scan(&imageIndex); err != nil {
			return err
		}
	}
	if v.ImageIndex != nil {
		imageIndex = *v.ImageIndex
	}
	if imageIndex < -1 || imageIndex > 5 {
		return bad("image_index: от -1 до 5")
	}
	sizes, colors := v.Attributes["size"], v.Attributes["color"]
	type variant struct{ size, color string }
	combinations := []variant{}
	for _, size := range sizes {
		for _, color := range colors {
			combinations = append(combinations, variant{size, color})
		}
	}
	if id != "" {
		var oldSize, oldColor string
		if err = tx.QueryRow("SELECT size,color FROM products WHERE id=?", id).Scan(&oldSize, &oldColor); err != nil {
			return err
		}
		for i, c := range combinations {
			if c.size == oldSize && c.color == oldColor {
				combinations[0], combinations[i] = combinations[i], combinations[0]
				break
			}
		}
	} else {
		code = 201
	}
	baseID := id
	ids := []string{}
	for i, c := range combinations {
		variantID := id
		existing := ""
		err = tx.QueryRow("SELECT id FROM products WHERE deleted=0 AND name=? AND brand=? AND size=? AND color=? AND id<>? LIMIT 1", v.Name, v.Brand, c.size, c.color, id).Scan(&existing)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if existing != "" && code == 201 {
			return apiError{409, "Товар с таким названием, размером и цветом уже существует"}
		}
		if i > 0 {
			variantID = existing
		}
		if i == 0 && existing != "" {
			return apiError{409, "Такой размер и цвет уже есть у этого товара"}
		}
		created := variantID == ""
		attrs := attributeJSON(v.Attributes, c.size, c.color)
		args := []any{v.Name, v.Type, v.Gender, c.size, c.color, v.Description, v.Brand, v.Season, v.Price, imageIndex, attrs}
		if created {
			variantID = store.ID()
			_, err = tx.Exec(`INSERT INTO products(name,type,gender,size,color,description,brand,season,price,image_index,attributes,id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, append(args, variantID)...)
		} else {
			err = affected(tx.Exec(`UPDATE products SET name=?,type=?,gender=?,size=?,color=?,description=?,brand=?,season=?,price=?,image_index=?,attributes=? WHERE id=? AND deleted=0`, append(args, variantID)...))
		}
		if err != nil {
			return err
		}
		if created && v.Stocks != nil {
			qty := v.Stocks[c.size][c.color]
			v.Stock = &qty
		}
		if created && v.Stock != nil {
			if _, err = tx.Exec("UPDATE products SET stock=? WHERE id=?", *v.Stock, variantID); err != nil {
				return err
			}
		}
		if i == 0 {
			baseID = variantID
		}
		if i > 0 && created {
			originalPhotos, e := rowsJSON(tx, "SELECT filename,mime_type FROM photos WHERE product_id=?", baseID)
			if e != nil {
				return e
			}
			for _, photo := range originalPhotos {
				if _, err = tx.Exec("INSERT INTO photos(id,filename,product_id,mime_type) VALUES(?,?,?,?)", store.ID(), photo["filename"], variantID, photo["mime_type"]); err != nil {
					return err
				}
			}
		} else {
			for _, photo := range saved {
				if _, err = tx.Exec("INSERT INTO photos(id,filename,product_id,mime_type) VALUES(?,?,?,?)", store.ID(), photo.Filename, variantID, photo.mime); err != nil {
					return err
				}
			}
		}
		ids = append(ids, variantID)
	}
	id = baseID
	if err = tx.Commit(); err != nil {
		return err
	}
	committed = true
	p, err := a.one(productSelect+" WHERE p.id=?", id)
	if err != nil {
		return err
	}
	photos, err := rowsJSON(a.db, "SELECT id,filename,product_id FROM photos WHERE product_id=? ORDER BY rowid", id)
	if err != nil {
		return err
	}
	p["photos"] = photos
	p["variant_ids"] = ids
	return respond(w, code, p)
}
func (a *App) deleteProduct(w http.ResponseWriter, r *http.Request) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	id := r.PathValue("id")
	if err = affected(tx.Exec("UPDATE products SET deleted=1 WHERE id=? AND deleted=0", id)); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM carts WHERE product_id=?", id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return respond(w, 200, map[string]bool{"ok": true})
}

type uploadedPhoto struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	ProductID string `json:"product_id"`
	mime      string
}

func (a *App) uploadPhoto(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return bad("Неверная загрузка или запрос больше 32 МБ")
	}
	defer r.MultipartForm.RemoveAll()
	files := r.MultipartForm.File["file"]
	if len(files) < 1 || len(files) > 10 {
		return bad("Выберите от 1 до 10 фотографий в поле file")
	}
	pid := r.FormValue("product_id")
	saved := []uploadedPhoto{}
	committed := false
	defer func() {
		if !committed {
			for _, photo := range saved {
				os.Remove(filepath.Join(a.cfg.UploadDir, photo.Filename))
			}
		}
	}()
	for _, header := range files {
		photo, err := a.savePhotoFile(header)
		if err != nil {
			return err
		}
		photo.ProductID = pid
		saved = append(saved, photo)
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRow("SELECT 1 FROM products WHERE id=? AND deleted=0", pid).Scan(&exists); err != nil {
		return err
	}
	for _, photo := range saved {
		if _, err = tx.Exec("INSERT INTO photos(id,filename,product_id,mime_type) VALUES(?,?,?,?)", photo.ID, photo.Filename, pid, photo.mime); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	committed = true
	if len(saved) == 1 {
		return respond(w, 201, saved[0])
	}
	return respond(w, 201, map[string]any{"items": saved, "total": len(saved)})
}

func (a *App) savePhotoFile(header *multipart.FileHeader) (uploadedPhoto, error) {
	if header.Size > 8<<20 {
		return uploadedPhoto{}, bad("Каждая фотография должна быть не больше 8 МБ")
	}
	file, err := header.Open()
	if err != nil {
		return uploadedPhoto{}, err
	}
	defer file.Close()
	cfg, format, err := image.DecodeConfig(file)
	if err != nil || (format != "jpeg" && format != "png") {
		return uploadedPhoto{}, bad("Разрешены изображения JPEG и PNG")
	}
	if cfg.Width > 8192 || cfg.Height > 8192 {
		return uploadedPhoto{}, bad("Разрешение не должно превышать 8192 × 8192")
	}
	if _, err = file.Seek(0, 0); err != nil {
		return uploadedPhoto{}, err
	}
	id := store.ID()
	ext, mime := ".jpg", "image/jpeg"
	if format == "png" {
		ext, mime = ".png", "image/png"
	}
	photo := uploadedPhoto{ID: id, Filename: id + ext, mime: mime}
	path := filepath.Join(a.cfg.UploadDir, photo.Filename)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return uploadedPhoto{}, err
	}
	_, copyErr := io.Copy(out, file)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(path)
		return uploadedPhoto{}, fmt.Errorf("save upload: %v %v", copyErr, closeErr)
	}
	return photo, nil
}
func (a *App) photo(w http.ResponseWriter, r *http.Request) error {
	var name, mime string
	if err := a.db.QueryRow("SELECT filename,mime_type FROM photos WHERE id=?", r.PathValue("id")).Scan(&name, &mime); err != nil {
		return err
	}
	f, err := os.Open(filepath.Join(a.cfg.UploadDir, filepath.Base(name)))
	if os.IsNotExist(err) {
		return missing()
	}
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", mime)
	http.ServeContent(w, r, name, st.ModTime(), f)
	return nil
}
func (a *App) deletePhoto(w http.ResponseWriter, r *http.Request) error {
	var name string
	if err := a.db.QueryRow("SELECT filename FROM photos WHERE id=?", r.PathValue("id")).Scan(&name); err != nil {
		return err
	}
	if err := affected(a.db.Exec("DELETE FROM photos WHERE id=?", r.PathValue("id"))); err != nil {
		return err
	}
	var references int
	if err := a.db.QueryRow("SELECT count(*) FROM photos WHERE filename=?", name).Scan(&references); err != nil {
		return err
	}
	if references > 0 {
		return respond(w, 200, map[string]bool{"ok": true})
	}
	if err := os.Remove(filepath.Join(a.cfg.UploadDir, filepath.Base(name))); err != nil && !os.IsNotExist(err) {
		a.log.Warn("orphan upload file", "filename", name, "error", err)
	}
	return respond(w, 200, map[string]bool{"ok": true})
}
