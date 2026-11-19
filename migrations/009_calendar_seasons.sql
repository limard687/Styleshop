CREATE TEMP TABLE calendar_season_upgrade AS
SELECT p.id,
 (SELECT json_group_array(calendar.value)
  FROM json_each('["Весна","Лето","Осень","Зима"]') calendar
  WHERE EXISTS (
   SELECT 1 FROM json_each(
    CASE WHEN json_array_length(json_extract(p.attributes, '$.season')) > 0
     THEN json_extract(p.attributes, '$.season') ELSE json_array(p.season) END
   ) old
   WHERE casefold(trim(old.value)) = casefold(calendar.value)
    OR casefold(trim(old.value)) IN ('всесезон', 'всесезонный')
    OR (casefold(trim(old.value)) IN ('демисезон', 'демисезонный', 'межсезонье')
        AND calendar.value IN ('Весна', 'Осень'))
  )) AS seasons
FROM products p;

UPDATE calendar_season_upgrade SET seasons = '["Весна","Осень"]' WHERE seasons = '[]';
UPDATE products SET
 season = (SELECT group_concat(value, ', ') FROM json_each(mapped.seasons)),
 attributes = json_set(products.attributes, '$.season', json(mapped.seasons))
FROM calendar_season_upgrade mapped WHERE products.id = mapped.id;
DROP TABLE calendar_season_upgrade;
