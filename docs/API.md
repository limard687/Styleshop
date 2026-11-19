# HTTP API StyleShop

Запросы JSON требуют `Content-Type: application/json`. Ошибки возвращаются как `{"error":"Сообщение"}`. Списки товаров, пользователей и заказов содержат `items`, `total`, `page`, `limit`. Пагинация: `page` от 1, `limit` от 1 до 100.

## 27 методов задания

| Метод | Путь | Доступ |
|---|---|---|
| POST | `/product` | администратор |
| GET | `/product` | Все |
| GET | `/product/{id}` | Все |
| PUT | `/product/{id}` | администратор |
| DELETE | `/product/{id}` | администратор |
| POST | `/photo` | администратор |
| GET | `/photo/{id}` | Все |
| DELETE | `/photo/{id}` | администратор |
| POST | `/role` | администратор |
| GET | `/role` | администратор |
| PUT | `/role/{id}` | администратор |
| DELETE | `/role/{id}` | администратор |
| POST | `/user` | Все |
| GET | `/user` | администратор |
| GET | `/user/role/{id}` | администратор |
| PUT | `/user/{id}` | Любой вошедший |
| DELETE | `/user/{id}` | администратор |
| POST | `/cart` | покупатель |
| GET | `/cart/{user_id}` | покупатель |
| PUT | `/cart/{id}` | покупатель |
| DELETE | `/cart/{id}` | покупатель |
| POST | `/order` | покупатель |
| GET | `/order` | покупатель, менеджер, администратор |
| GET | `/order/{id}` | покупатель, менеджер, администратор |
| PUT | `/order/{id}` | покупатель, менеджер, администратор |
| POST | `/order_item` | администратор |
| GET | `/order_item/{order_id}` | покупатель, менеджер, администратор |

При регистрации `POST /user` создаёт покупателя; назначать другую роль может администратор. Покупатель читает и меняет только свою корзину и свой профиль, видит только свои заказы. Отменять свой заказ можно до статуса «Готов к выдаче». Менеджер и администратор видят все заказы.

## Дополнительные маршруты

| Метод | Путь | Доступ |
|---|---|---|
| GET | `/health` | Все |
| POST | `/auth/login` | Все |
| POST | `/auth/logout` | Любой вошедший |
| GET | `/auth/me` | Любой вошедший |
| PATCH | `/auth/me` | Любой вошедший |
| PATCH | `/product/{id}/stock` | администратор |
| GET | `/product/options` | Все |
| POST | `/cart/batch` | покупатель |

## Примеры

Локальный вход с сохранением cookie:

```sh
curl -c cookies.txt -X POST http://127.0.0.1:8082/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"client@styleshop.local","password":"StyleDemo2026!"}'
```

Ответ входа также содержит `token`: его можно передавать как `Authorization: Bearer <token>`. Обычный интерфейс использует HttpOnly cookie. Выход `POST /auth/logout` отзывает текущую сессию.

```sh
curl -b cookies.txt http://127.0.0.1:8082/auth/me
curl 'http://127.0.0.1:8082/product?brand=boSS&size=XL&sort=price_asc&page=1&limit=12'
curl -b cookies.txt -X POST http://127.0.0.1:8082/cart \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"e87072c3-6590-45e9-9586-9f9e852227c9","count":1}'
curl -b cookies.txt http://127.0.0.1:8082/cart/10000000-0000-4000-8000-000000000001
curl -b cookies.txt -X POST http://127.0.0.1:8082/order \
  -H 'Content-Type: application/json' -d '{}'
curl -b cookies.txt http://127.0.0.1:8082/order
```

Для частичного оформления передайте `{"cart_ids":["ID строки корзины"]}` в `POST /order`. Пустой объект оформляет всю корзину. Нельзя передавать чужие или повторяющиеся строки.

Отмена заказа:

```sh
curl -b cookies.txt -X PUT http://127.0.0.1:8082/order/ORDER_ID \
  -H 'Content-Type: application/json' \
  -d '{"status":"Отменён","cancellation_reason":"Передумал"}'
```

Статусы: `Принят`, `В работе`, `Собран`, `Готов к выдаче`, `Отменён`. Для сотрудника причина отмены обязательна. Повторная отмена не возвращает остатки дважды.

Регистрация:

```json
{"name":"Покупатель","email":"buyer@example.com","phone":"+79001234567","password":"Password2026!","password_confirmation":"Password2026!","privacy_accepted":true}
```

Создание товара администратором (цена в копейках):

```json
{"name":"Футболка","description":"Хлопковая футболка","price":199000,"attributes":{"type":["Футболки"],"gender":["Унисекс"],"size":["S","M"],"color":["Белый"],"brand":["StyleShop"],"season":["Лето"]},"stock_by_variant":{"S":{"Белый":5},"M":{"Белый":8}}}
```

Для создания товара одновременно с фото используйте multipart: `product` с JSON и повторяющиеся поля `file`. `POST /photo` принимает `product_id` и один или несколько `file` (PNG/JPEG, до 10 штук по 8 МБ). `GET /photo/{id}` возвращает само изображение; фотографии WebP из исходного каталога доступны для просмотра.

Изменение остатка: `PATCH /product/{id}/stock`, тело `{"stock":10,"expected_stock":7}`. Если другой запрос уже изменил остаток, сервер вернёт 409.
