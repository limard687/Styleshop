ALTER TABLE orders ADD COLUMN order_number INTEGER CHECK(order_number > 0);
WITH numbered AS (
 SELECT id, row_number() OVER (ORDER BY date_time, rowid) AS number FROM orders
)
UPDATE orders SET order_number = (SELECT number FROM numbered WHERE numbered.id = orders.id);
CREATE UNIQUE INDEX idx_orders_number ON orders(order_number);
