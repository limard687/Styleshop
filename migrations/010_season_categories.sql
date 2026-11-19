CREATE TEMP TABLE season_category_upgrade AS
SELECT p.id,
 (SELECT json_group_array(category.value)
  FROM json_each('["Лето","Зима","Демисезон"]') category
  WHERE EXISTS (
   SELECT 1 FROM json_each(
    CASE WHEN json_array_length(json_extract(p.attributes, '$.season')) > 0
     THEN json_extract(p.attributes, '$.season') ELSE json_array(p.season) END
   ) old
   WHERE casefold(trim(old.value)) = casefold(category.value)
    OR casefold(trim(old.value)) IN ('всесезон', 'всесезонный')
    OR (casefold(trim(old.value)) IN ('весна', 'осень', 'демисезонный', 'межсезонье')
        AND category.value = 'Демисезон')
  )) AS seasons
FROM products p;

UPDATE season_category_upgrade SET seasons = '["Демисезон"]' WHERE seasons = '[]';
UPDATE products SET
 season = (SELECT group_concat(value, ', ') FROM json_each(mapped.seasons)),
 attributes = json_set(products.attributes, '$.season', json(mapped.seasons))
FROM season_category_upgrade mapped WHERE products.id = mapped.id;
DROP TABLE season_category_upgrade;
