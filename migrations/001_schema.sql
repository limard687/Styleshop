PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS roles (
 id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS users (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL UNIQUE COLLATE NOCASE,
 phone TEXT NOT NULL DEFAULT '', role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
 registration_date TEXT NOT NULL, password_hash TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS products (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL, gender TEXT NOT NULL,
 size TEXT NOT NULL, color TEXT NOT NULL, description TEXT NOT NULL, brand TEXT NOT NULL,
 season TEXT NOT NULL, price INTEGER NOT NULL CHECK(price >= 0),
 image_index INTEGER NOT NULL DEFAULT -1, deleted INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS photos (
 id TEXT PRIMARY KEY, filename TEXT NOT NULL, product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
 mime_type TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS carts (
 id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
 count INTEGER NOT NULL CHECK(count BETWEEN 1 AND 99), UNIQUE(user_id, product_id)
);
CREATE TABLE IF NOT EXISTS orders (
 id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 date_time TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'Принят'
 CHECK(status IN ('Принят','В работе','Собран','Готов к выдаче','Отменён'))
);
CREATE TABLE IF NOT EXISTS order_items (
 id TEXT PRIMARY KEY, order_id TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
 product_id TEXT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
 price INTEGER NOT NULL CHECK(price >= 0), count INTEGER NOT NULL DEFAULT 1 CHECK(count BETWEEN 1 AND 99)
);
CREATE TABLE IF NOT EXISTS sessions (
 token_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_products_filter ON products(deleted, type, gender, size, color, brand, season, price);
CREATE INDEX IF NOT EXISTS idx_orders_user_date ON orders(user_id, date_time);
CREATE INDEX IF NOT EXISTS idx_orders_status_date ON orders(status, date_time);
CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items(order_id);
CREATE INDEX IF NOT EXISTS idx_photos_product ON photos(product_id);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role_id);
