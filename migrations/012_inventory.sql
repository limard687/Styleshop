ALTER TABLE products ADD COLUMN stock INTEGER NOT NULL DEFAULT 20 CHECK(stock >= 0);
ALTER TABLE order_items ADD COLUMN reserved_count INTEGER NOT NULL DEFAULT 0 CHECK(reserved_count >= 0);
