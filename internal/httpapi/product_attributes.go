package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"
)

var attributeLimits = map[string]int{"type": 120, "gender": 120, "size": 20, "color": 60, "brand": 120, "season": 120}
var productSeasons = []string{"Лето", "Зима", "Демисезон", "Круглогодичный"}

func normalizeAttributes(v *productInput) error {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || utf8.RuneCountInString(v.Name) > 120 {
		return bad("Название: от 1 до 120 символов")
	}
	fields := map[string]*string{"type": &v.Type, "gender": &v.Gender, "size": &v.Size, "color": &v.Color, "brand": &v.Brand, "season": &v.Season}
	if v.Attributes == nil {
		v.Attributes = map[string][]string{}
	}
	for key := range v.Attributes {
		if _, ok := fields[key]; !ok {
			return bad("Неизвестная характеристика товара")
		}
	}
	for key, field := range fields {
		values, provided := v.Attributes[key]
		if !provided {
			values = []string{*field}
		}
		if len(values) == 0 || len(values) > 10 {
			return bad("Выберите от 1 до 10 значений каждой характеристики")
		}
		cleaned := []string{}
		seen := map[string]bool{}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if key == "season" {
				if strings.EqualFold(value, "Демисезонный") {
					value = "Демисезон"
				}
				canonical := ""
				for _, season := range productSeasons {
					if strings.EqualFold(value, season) {
						canonical = season
						break
					}
				}
				if canonical == "" {
					return bad("Выберите сезон: Лето, Зима, Демисезон (весна–осень) или Круглогодичный")
				}
				value = canonical
			}
			if value == "" || utf8.RuneCountInString(value) > attributeLimits[key] {
				return bad("Значение характеристики пустое или слишком длинное")
			}
			if strings.ContainsAny(value, "\n\r\t") {
				return bad("Характеристика должна быть в одну строку")
			}
			lower := strings.ToLower(value)
			if !seen[lower] {
				cleaned = append(cleaned, value)
				seen[lower] = true
			}
		}
		v.Attributes[key] = cleaned
		*field = strings.Join(cleaned, ", ")
	}
	if len(v.Attributes["size"])*len(v.Attributes["color"]) > 25 {
		return bad("За один раз можно сохранить до 25 сочетаний размеров и цветов")
	}
	return nil
}

func (a *App) productOptions(w http.ResponseWriter, r *http.Request) error {
	items, err := rowsJSON(a.db, `SELECT type,gender,size,color,brand,season,attributes FROM products WHERE deleted=0`)
	if err != nil {
		return err
	}
	result := map[string][]string{"type": {}, "gender": {"Мужской", "Женский", "Унисекс"}, "size": {}, "color": {}, "brand": {}, "season": productSeasons}
	for key, values := range result {
		if key == "season" {
			continue
		}
		seen := map[string]bool{}
		for _, v := range values {
			seen[strings.ToLower(v)] = true
		}
		for _, item := range items {
			values := []string{item[key].(string)}
			if attrs, ok := item["attributes"].(map[string][]string); ok && len(attrs[key]) > 0 {
				values = attrs[key]
			}
			for _, v := range values {
				if v != "" && !seen[strings.ToLower(v)] {
					result[key] = append(result[key], v)
					seen[strings.ToLower(v)] = true
				}
			}
		}
		if key != "gender" && key != "season" {
			sort.Slice(result[key], func(i, j int) bool { return strings.ToLower(result[key][i]) < strings.ToLower(result[key][j]) })
		}
	}
	return respond(w, 200, result)
}

func attributeJSON(attrs map[string][]string, size, color string) string {
	out := map[string][]string{}
	for k, v := range attrs {
		out[k] = v
	}
	out["size"] = []string{size}
	out["color"] = []string{color}
	data, _ := json.Marshal(out)
	return string(data)
}
