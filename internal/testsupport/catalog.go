package testsupport

import (
	"clothing-store/internal/store"
	"database/sql"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"time"
)

// Seed creates a synthetic catalog used exclusively in integration tests.
func Seed(db *sql.DB, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, role := range []string{"client", "manager", "admin"} {
		name := []string{"Покупатель", "Менеджер", "Администратор"}[i]
		_, err = tx.Exec(`INSERT OR IGNORE INTO users(id,name,email,phone,role_id,registration_date,password_hash) VALUES(?,?,?,?,?,?,?)`, fmt.Sprintf("10000000-0000-4000-8000-%012d", i+1), name, role+"@styleshop.local", "", fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1), time.Now().UTC().Format(time.RFC3339), string(hash))
		if err != nil {
			return err
		}
	}
	var n int
	if err = tx.QueryRow("SELECT count(*) FROM products").Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		types := []string{"Худи", "Футболки", "Брюки", "Рубашки", "Куртки", "Обувь"}
		names := []string{"Худи «Уголь»", "Футболка «Камень»", "Брюки «Графит»", "Рубашка «Мел»", "Бомбер «Пепел»", "Слипоны «Карбон»"}
		colors := []string{"Угольный", "Каменный", "Чёрный", "Меловой", "Пепельный", "Чёрный"}
		desc := []string{"Объёмное худи из плотного хлопкового футера. Спущенное плечо, двойной капюшон без шнурков, карман спереди. Состав: 100% хлопок. Стирка при 30 °C.", "Прямая футболка с широким рукавом и плотной горловиной. Мягкий хлопковый джерси, слегка укороченная посадка. Состав: 100% хлопок. Стирка при 30 °C.", "Свободные брюки с эластичным поясом и двумя боковыми карманами. Плотный футер с эффектом выстиранной ткани. Состав: 100% хлопок. Стирка при 30 °C.", "Рубашка свободного кроя из плотного хлопка. Отложной воротник, нагрудный карман, застёжка на пуговицы. Состав: 100% хлопок. Стирка при 30 °C.", "Укороченный бомбер с объёмным силуэтом. Застёжка на молнию, эластичные манжеты, утеплённая подкладка. Верх: 100% полиэстер. Деликатная чистка.", "Лёгкие слипоны цельной формы. Мягкий вспененный материал, широкая колодка, рельефная подошва. Материал: EVA. Очищать влажной тканью."}
		prices := []int{7900, 3200, 6400, 4200, 12900, 4900}
		for i := range names {
			sizes := []string{"S", "M", "L"}
			if i == 5 {
				sizes = []string{"40", "42", "44"}
			}
			for _, size := range sizes {
				_, err = tx.Exec(`INSERT INTO products(id,name,type,gender,size,color,description,brand,season,price,image_index) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, store.ID(), names[i], types[i], []string{"Унисекс", "Унисекс", "Мужской", "Женский", "Мужской", "Унисекс"}[i], size, colors[i], desc[i], "StyleShop", []string{"Зима", "Лето", "Демисезон", "Демисезон", "Демисезон", "Лето"}[i], prices[i]*100, i)
				if err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}
