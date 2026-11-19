ALTER TABLE users ADD COLUMN privacy_accepted_at TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN privacy_version TEXT NOT NULL DEFAULT '';

UPDATE products SET season = CASE
 WHEN season = 'Демисезон' THEN 'Демисезонный'
 WHEN season = 'Всесезон' AND brand = 'FORM' AND image_index = 0 THEN 'Зима'
 WHEN season = 'Всесезон' AND brand = 'FORM' AND image_index = 1 THEN 'Лето'
 WHEN season = 'Всесезон' AND brand = 'FORM' AND image_index = 2 THEN 'Осень'
 WHEN season = 'Всесезон' AND brand = 'FORM' AND image_index = 3 THEN 'Весна'
 ELSE 'Демисезонный' END
WHERE season IN ('Всесезон', 'Демисезон');
