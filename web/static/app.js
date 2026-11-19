"use strict";

const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];
const esc = (v) =>
  String(v ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
const money = (value) =>
  new Intl.NumberFormat("ru-RU", {
    style: "currency",
    currency: "RUB",
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  }).format(value / 100);
const date = (value) =>
  new Date(value).toLocaleString("ru-RU", {
    dateStyle: "short",
    timeStyle: "short",
  });
const state = {
  user: null,
  groups: [],
  variants: [],
  selected: null,
  tab: "orders",
  page: 1,
  cart: [],
  cartTotal: 0,
  selectedCartIDs: null,
  roles: [],
  orderFilters: {},
  inventoryQuery: "",
  request: 0,
};
const modal = $("#modal");
let toastTimer;
const productPhotoQueue = new WeakMap();

async function api(path, { method = "GET", body, form } = {}) {
  const headers = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const response = await fetch(path, {
    method,
    headers,
    credentials: "same-origin",
    body: form ?? (body === undefined ? undefined : JSON.stringify(body)),
  });
  const data = await response
    .json()
    .catch(() => ({ error: "Сервер вернул неожиданный ответ" }));
  if (!response.ok) {
    const e = new Error(data.error || "Не удалось выполнить запрос");
    e.status = response.status;
    throw e;
  }
  return data;
}
function toast(message) {
  clearTimeout(toastTimer);
  const notice = $("#toast");
  notice.textContent = message;
  notice.hidden = false;
  if (typeof notice.showPopover === "function") {
    if (notice.matches(":popover-open")) notice.hidePopover();
    notice.showPopover();
  } else {
    const dialogs = $$("dialog[open]");
    (dialogs.at(-1) || document.body).appendChild(notice);
  }
  toastTimer = setTimeout(() => {
    if (typeof notice.hidePopover === "function" && notice.matches(":popover-open")) notice.hidePopover();
    notice.hidden = true;
  }, 4500);
}
function showModal(title, content, kind = "compact") {
  clearProductPhotoQueue($("#product-form"));
  modal.className = kind;
  $("#modal-content").innerHTML =
    `<div class="modal-header"><h2 id="modal-title">${esc(title)}</h2><button class="close" data-action="close" aria-label="Закрыть">×</button></div>${content}`;
  if (!modal.open) modal.showModal();
}
function errorMarkup(message) {
  return `<p class="form-error" role="alert">${esc(message)}</p>`;
}
function picture(p, extra = "") {
  const label = esc(`${p.name}, ${p.color || ""}`);
  if (p.photo_id)
    return `<div class="product-image uploaded ${extra}"><img src="/photo/${encodeURIComponent(p.photo_id)}" alt="${label}" loading="lazy"></div>`;
  const i = Number(p.image_index);
  if (Number.isInteger(i) && i >= 0 && i <= 5)
    return `<div class="product-image pic${i} ${extra}" role="img" aria-label="${label}"></div>`;
  return `<div class="product-image no-photo ${extra}" role="img" aria-label="${label}: фото не добавлено">НЕТ ФОТО</div>`;
}
function roleLabel(role) {
  return (
    { client: "Покупатель", manager: "Менеджер", admin: "Администратор" }[
      role
    ] || role
  );
}
function attributeValueLabel(key, value) {
  return key === "season" && value === "Демисезон"
    ? "Демисезон (весна–осень)"
    : value;
}
function accountNav() {
  const u = state.user;
  $("#account-nav").innerHTML =
    `${u ? `<button class="nav-link" data-action="workspace" data-tab="orders">${u.role === "client" ? "Заказы" : "Панель"}</button>` : ""}<button class="nav-link account-button" data-action="${u ? "account" : "login"}" ${u ? `title="${esc(u.name)} / ${esc(u.email)} / ${esc(roleLabel(u.role))}"` : ""}>${u ? `<span class="account-name">${esc(u.name.toLowerCase() === roleLabel(u.role).toLowerCase() ? u.email.split("@")[0] : u.name)}</span><span class="account-role">${esc(roleLabel(u.role))}</span>` : "Войти"}</button>${!u || u.role === "client" ? `<button class="nav-link" data-action="cart">Корзина (${state.cart.reduce((sum, item) => sum + item.count, 0)})</button>` : ""}`;
}
function cartActionMarkup() {
  const restricted = state.user && state.user.role !== "client";
  return `<button class="primary" data-action="add-cart" ${restricted ? "disabled" : ""}>${restricted ? "Только для покупателей" : "Добавить в корзину"}</button>`;
}
async function loadCart() {
  if (state.user?.role === "client") {
    const c = await api(`/cart/${state.user.id}`);
    const previous = new Set(state.cart.map((item) => item.id));
    const selected = state.selectedCartIDs ?? new Set();
    for (const item of c.items)
      if (state.selectedCartIDs === null || !previous.has(item.id))
        selected.add(item.id);
    state.selectedCartIDs = new Set(
      c.items.filter((item) => selected.has(item.id)).map((item) => item.id),
    );
    state.cart = c.items;
    state.cartTotal = c.total;
  } else {
    state.cart = [];
    state.selectedCartIDs = null;
    state.cartTotal = 0;
  }
  accountNav();
}
async function loadProducts() {
  const request = ++state.request;
  const params = new URLSearchParams({ limit: "100" });
  for (const [key, value] of new FormData($("#filters")))
    if (String(value).trim()) params.append(key, String(value).trim());
  const data = await api("/product?" + params);
  if (!state.catalogOptions)
    state.catalogOptions = await api("/product/options");
  if (request !== state.request) return;
  state.variants = data.items;
  const grouped = new Map();
  for (const p of data.items) {
    const key = JSON.stringify([p.name, p.brand, p.color, p.type]);
    if (!grouped.has(key)) grouped.set(key, []);
    grouped.get(key).push(p);
  }
  const sizeRank = (v) =>
    ({ XS: 0, S: 1, M: 2, L: 3, XL: 4, XXL: 5, XXXL: 6, "4XL": 7 })[v] ??
    (Number(v) || 100);
  state.groups = [...grouped.values()].map((group) =>
    group.sort((a, b) => sizeRank(a.size) - sizeRank(b.size)),
  );
  updateFilterOptions(state.catalogOptions);
  $("#catalog-count").textContent =
    `${state.groups.length} ${plural(state.groups.length, "модель", "модели", "моделей")}`;
  $("#products").innerHTML = state.groups.length
    ? state.groups
        .map((group, i) => {
          const p = group[0];
          return `<button class="product-card" data-action="product" data-index="${i}" aria-label="Открыть ${esc(p.name)}">${picture(p)}<p class="card-brand">${esc(p.brand)}</p><div class="product-meta"><p>${esc(p.name)}</p><p>${money(p.price)}</p></div><p class="product-color">${esc(p.color)} / ${esc(group.map((v) => v.size).join(", "))}</p><span class="card-open">Подробнее</span></button>`;
        })
        .join("")
    : '<div class="empty"><p>Ничего не найдено.<br>Попробуй другой размер или убери часть фильтров.</p><button class="text-button" data-action="reset-filters">Сбросить фильтры</button></div>';
}
function updateFilterOptions(options) {
  const definitions = Object.fromEntries(
    Object.keys(attributeLabels).map((key) => [key, options[key] || []]),
  );
  for (const [key, fixed] of Object.entries(definitions)) {
    const fieldset = $(`[data-filter-options="${key}"]`);
    const selected = new Set(
      $$(`input[name="${key}"]:checked`, fieldset).map((input) => input.value),
    );
    let values = [...new Set([...fixed, ...selected])];
    if (key === "size") {
      const rank = (value) =>
        ({ XS: 0, S: 1, M: 2, L: 3, XL: 4, XXL: 5, XXXL: 6, "4XL": 7 })[
          value
        ] ??
        (Number(value) || 100);
      values.sort((a, b) => rank(a) - rank(b));
    }
    const label = $("legend", fieldset).outerHTML;
    fieldset.innerHTML =
      label +
      values
        .map(
          (value) =>
            `<label><input type="checkbox" name="${key}" aria-label="${esc(attributeValueLabel(key, value))}" value="${esc(value)}" ${selected.has(value) ? "checked" : ""}><span>${esc(attributeValueLabel(key, value))}</span></label>`,
        )
        .join("");
  }
  updateFilterSummaries();
}
function updateFilterSummaries() {
  for (const dropdown of $$(".filter-dropdown")) {
    const count = $$("input:checked", dropdown).length;
    const counter = $(".filter-selected", dropdown);
    counter.hidden = count === 0;
    counter.textContent = `(${count})`;
    dropdown.classList.toggle("has-selection", count > 0);
  }
}
$("#filters").addEventListener(
  "toggle",
  (event) => {
    if (!event.target.matches(".filter-dropdown") || !event.target.open) return;
    for (const other of $$(".filter-dropdown[open]"))
      if (other !== event.target) other.open = false;
  },
  true,
);
document.addEventListener("click", (event) => {
  for (const dropdown of $$(".filter-dropdown[open]"))
    if (!dropdown.contains(event.target)) dropdown.open = false;
});
document.addEventListener("keydown", (event) => {
  if (event.key !== "Escape") return;
  const open = $(".filter-dropdown[open]");
  if (open) {
    open.open = false;
    $("summary", open).focus();
  }
});
function plural(n, a, b, c) {
  return n % 10 === 1 && n % 100 !== 11
    ? a
    : n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 12 || n % 100 > 14)
      ? b
      : c;
}
function galleryCount(p) {
  return Math.max(1, p.photos?.length || 0);
}
function galleryPicture(p, index) {
  return picture({
    ...p,
    photo_id: p.photos?.length ? p.photos[index].id : null,
  });
}
function galleryMarkup(p) {
  const count = galleryCount(p);
  return `<section class="product-gallery" aria-label="Фотографии товара">
    <button class="gallery-open" data-action="gallery-enlarge" aria-label="Увеличить фотографию товара">${galleryPicture(p, 0)}</button>
    <div class="gallery-toolbar"><button data-action="gallery-prev" aria-label="Предыдущее фото" ${count === 1 ? "disabled" : ""}>←</button><span class="mono gallery-counter" aria-live="polite">Фото 1 из ${count}</span><button data-action="gallery-next" aria-label="Следующее фото" ${count === 1 ? "disabled" : ""}>→</button></div>
    ${count > 1 ? `<div class="gallery-thumbnails" role="group" aria-label="Выбрать фотографию">${p.photos.map((photo, i) => `<button class="gallery-thumb" data-action="gallery-select" data-index="${i}" aria-label="Фото ${i + 1}" aria-pressed="${i === 0}">${galleryPicture(p, i)}</button>`).join("")}</div>` : ""}
  </section>`;
}
function changeGallery(index) {
  const selected = state.selected;
  if (!selected) return;
  const count = galleryCount(selected.product);
  selected.galleryIndex = (index + count) % count;
  const image = galleryPicture(selected.product, selected.galleryIndex);
  $(".gallery-open", modal).innerHTML = image;
  $(".gallery-counter", modal).textContent =
    `Фото ${selected.galleryIndex + 1} из ${count}`;
  for (const thumb of $$(".gallery-thumb", modal))
    thumb.setAttribute(
      "aria-pressed",
      String(Number(thumb.dataset.index) === selected.galleryIndex),
    );
  if (lightbox.open) {
    $(".lightbox-image", lightbox).innerHTML = image;
    $(".gallery-counter", lightbox).textContent =
      `Фото ${selected.galleryIndex + 1} из ${count}`;
  }
}
const lightbox = document.createElement("dialog");
lightbox.id = "lightbox";
lightbox.className = "lightbox";
lightbox.setAttribute("aria-labelledby", "lightbox-title");
document.body.append(lightbox);
function enlargeGallery() {
  const selected = state.selected;
  const count = galleryCount(selected.product);
  lightbox.innerHTML = `<div class="modal-header"><h2 id="lightbox-title">${esc(selected.product.name)}</h2><button class="close" data-action="lightbox-close" aria-label="Закрыть увеличенное фото">×</button></div><div class="lightbox-image">${galleryPicture(selected.product, selected.galleryIndex)}</div><div class="gallery-toolbar"><button data-action="gallery-prev" aria-label="Предыдущее фото" ${count === 1 ? "disabled" : ""}>←</button><span class="mono gallery-counter" aria-live="polite">Фото ${selected.galleryIndex + 1} из ${count}</span><button data-action="gallery-next" aria-label="Следующее фото" ${count === 1 ? "disabled" : ""}>→</button></div>`;
  lightbox.showModal();
}
document.addEventListener("keydown", (event) => {
  if (
    !modal.open ||
    !modal.classList.contains("product-modal") ||
    /INPUT|TEXTAREA|SELECT/.test(event.target.tagName)
  )
    return;
  if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
    event.preventDefault();
    changeGallery(
      state.selected.galleryIndex + (event.key === "ArrowLeft" ? -1 : 1),
    );
  }
});
async function productModal(index) {
  const variants = state.groups[index];
  if (!variants) return;
  const p = await api("/product/" + variants[0].id);
  showProductModal(p, variants);
}
async function productById(id) {
  const p = await api("/product/" + encodeURIComponent(id));
  const data = await api(
    "/product?" + new URLSearchParams({ q: p.name, limit: "100" }),
  );
  const variants = data.items.filter(
    (v) =>
      v.name === p.name &&
      v.brand === p.brand &&
      v.color === p.color &&
      v.type === p.type,
  );
  showProductModal(p, [p, ...variants.filter((v) => v.id !== p.id)]);
}
function showProductModal(p, variants) {
  state.selected = { variants, product: p, id: p.id, galleryIndex: 0 };
  showModal(
    p.name,
    `<div class="product-detail">${galleryMarkup(p)}<div class="detail-info"><div><h3>${esc(p.name)}</h3><p class="price" id="detail-price">${money(p.price)}</p></div><p class="description">${esc(p.description)}</p><div class="detail-specs">${esc(p.color)}<br>${esc(p.gender)} / ${esc(p.season)}<br>Бренд ${esc(p.brand)}<br><span id="detail-stock">На складе: ${p.stock} шт.</span></div><div><p class="mono">Размер</p><div class="sizes" role="group" aria-label="Размер товара">${variants.map(v => `<button class="size" data-action="size" data-id="${esc(v.id)}" aria-pressed="${v.id===p.id}">${esc(v.size)}</button>`).join("")}</div></div><label class="purchase-quantity">Количество<input id="purchase-quantity" class="stock-quantity" type="number" min="1" max="99" step="1" value="1"></label>${cartActionMarkup()}</div></div>`,
    "product-modal",
  );
  syncPurchaseQuantity();
}
function syncPurchaseQuantity(reset = true) {
  const input = $("#purchase-quantity");
  if (!input || !state.selected) return;
  const p = state.selected.product;
  const inCart = state.cart.find(item=>item.product_id===p.id)?.count || 0;
  const available = Math.max(0,Math.min(99-inCart,p.stock-inCart));
  input.max = String(available);
  input.value = available ? String(reset ? 1 : Math.min(available, Math.max(1, Number(input.value) || 1))) : "0";
  input.disabled = available===0 || Boolean(state.user && state.user.role!=="client");
  const button = $('[data-action="add-cart"]', modal);
  button.disabled = input.disabled;
  button.textContent = state.user && state.user.role!=="client" ? "Только для покупателей" : available===0 ? "Нет доступного количества" : "Добавить в корзину";
}
function showPrivacy() {
  const dialog = $("#privacy-modal");
  dialog.innerHTML = `<div class="modal-header"><h2 id="privacy-title">Политика конфиденциальности</h2><button class="close" data-action="privacy-close" aria-label="Закрыть политику">×</button></div><div class="modal-body policy-copy">${$("#privacy-text").innerHTML}</div>`;
  dialog.showModal();
}
function authModal(register = false) {
  showModal(
    register ? "Создать аккаунт" : "Войти",
    `<div class="modal-body"><div class="auth-tabs"><button class="${register ? "" : "active"}" data-action="login">Вход</button><button class="${register ? "active" : ""}" data-action="register">Регистрация</button></div><form id="auth-form" class="form-stack" data-register="${register}">${register ? field("Имя", "name") + field("Телефон", "phone", "", "tel") : ""}${field("Email", "email", "", "email")}<label>Пароль<input name="password" type="password" minlength="8" maxlength="72" autocomplete="${register ? "new-password" : "current-password"}" required></label>${register ? '<p class="field-help">Минимум 8 символов.</p><label>Повторите пароль<input name="password_confirmation" type="password" minlength="8" maxlength="72" autocomplete="new-password" required></label><div class="consent-row"><label class="consent-check"><input name="privacy_accepted" type="checkbox" value="true" required><span>Согласен с политикой конфиденциальности и обработкой данных аккаунта.</span></label><button class="text-button" data-action="privacy" type="button">Прочитать политику</button></div>' : ""}<p class="form-error" role="alert"></p><button class="primary" type="submit">${register ? "Создать аккаунт" : "Войти"}</button></form></div>`,
  );
}
function chosenCartItems() {
  return state.cart.filter((item) => state.selectedCartIDs?.has(item.id));
}
function updateCartSelection() {
  const chosen = chosenCartItems();
  $("#selected-cart-total").textContent = money(
    chosen.reduce((sum, item) => sum + item.price * item.count, 0),
  );
  $("#selected-cart-summary").textContent =
    `Выбрано ${chosen.length} из ${state.cart.length} позиций`;
  const all = $("#cart-select-all");
  all.checked = chosen.length === state.cart.length;
  all.indeterminate = chosen.length > 0 && chosen.length < state.cart.length;
  for (const input of $$("[data-cart-select]", modal))
    input.checked = state.selectedCartIDs.has(input.dataset.cartSelect);
  $('[data-action="checkout"]', modal).disabled = chosen.length === 0;
}
function cartModal() {
  if (!state.user) {
    authModal();
    toast("Войди, чтобы собрать корзину");
    return;
  }
  if (state.user.role !== "client") {
    toast("Корзина доступна только покупателям");
    return;
  }
  showModal(
    "Корзина",
    `<div class="modal-body">${state.cart.length ? `<div class="cart-selection"><label><input id="cart-select-all" type="checkbox"> Выбрать всё</label><span id="selected-cart-summary" aria-live="polite"></span></div>${state.cart.map((v) => `<article class="cart-line selectable"><label class="cart-selector"><input type="checkbox" data-cart-select="${v.id}" aria-label="Выбрать ${esc(v.name)}, размер ${esc(v.size)}"></label>${picture(v)}<div><h3>${esc(v.name)}</h3><p>${esc(v.color)} / ${esc(v.size)}</p><p>${money(v.price * v.count)}</p><div class="cart-controls"><button data-action="cart-count" data-id="${v.id}" data-count="${v.count - 1}" aria-label="Уменьшить количество ${esc(v.name)}">−</button><span>${v.count}</span><button data-action="cart-count" data-id="${v.id}" data-count="${v.count + 1}" aria-label="Увеличить количество ${esc(v.name)}" ${v.count >= 99 ? "disabled" : ""}>+</button><button data-action="cart-delete" data-id="${v.id}">Убрать</button></div></div></article>`).join("")}<div class="cart-total"><span>Итого за выбранное</span><span id="selected-cart-total" aria-live="polite"></span></div><button class="primary" data-action="checkout">Оформить выбранное</button>` : '<div class="empty"><p>Здесь пока пусто.</p><button class="text-button" data-action="close">Вернуться к коллекции ↗</button></div>'}</div>`,
    "drawer",
  );
  if (state.cart.length) updateCartSelection();
}
document.addEventListener("change", (event) => {
  if (event.target.matches("[data-cart-select]")) {
    const id = event.target.dataset.cartSelect;
    if (event.target.checked) state.selectedCartIDs.add(id);
    else state.selectedCartIDs.delete(id);
    updateCartSelection();
  } else if (event.target.id === "cart-select-all") {
    state.selectedCartIDs = new Set(
      event.target.checked ? state.cart.map((v) => v.id) : [],
    );
    updateCartSelection();
  } else if (
    event.target.matches(
      '#status-form [name="status"], #status-form [name="cancellation_reason_choice"]',
    )
  ) {
    syncCancellationForm(event.target.form);
  }
});
function syncCancellationForm(form) {
  const cancel = form.elements.status.value === "Отменён";
  const choice = form.elements.cancellation_reason_choice;
  const custom = cancel && choice.value === "custom";
  $(".cancel-reason", form).hidden = !cancel;
  choice.disabled = !cancel;
  choice.required = cancel;
  $(".cancel-custom-reason", form).hidden = !custom;
  form.elements.cancellation_reason.disabled = !custom;
  form.elements.cancellation_reason.required = custom;
}
async function workspace(tab = state.tab, page = 1) {
  if (!state.user) {
    authModal();
    return;
  }
  state.tab = tab;
  state.page = page;
  $("#store-view").hidden = true;
  $("#workspace-view").hidden = false;
  const admin = state.user.role === "admin",
    staff = admin || state.user.role === "manager";
  const tabs = [
    ...(staff || state.user.role === "client"
      ? [["orders", staff ? "Заказы клиентов" : "Мои заказы"]]
      : []),
    ...(staff ? [["inventory", "Склад"]] : []),
    ...(admin
      ? [
          ["products", "Товары"],
          ["users", "Пользователи"],
          ["roles", "Роли"],
        ]
      : []),
    ["profile", "Профиль"],
  ];
  if (!tabs.some(([key]) => key === tab)) return workspace("profile");
  $("#workspace-title").textContent = tabs.find(([key]) => key === tab)[1];
  $("#workspace-nav").innerHTML = tabs
    .map(
      ([key, label]) =>
        `<button data-action="workspace" data-tab="${key}" class="${key === tab ? "active" : ""}" ${key === tab ? 'aria-current="page"' : ""}>${label}</button>`,
    )
    .join("");
  if (tab === "orders") await orderWorkspace(page);
  if (tab === "products") await productWorkspace(page);
  if (tab === "inventory") await inventoryWorkspace(page);
  if (tab === "users") await userWorkspace(page);
  if (tab === "roles") await roleWorkspace();
  if (tab === "profile") profileWorkspace();
}
function pager(data) {
  return `<div class="pagination"><button data-action="page" data-page="${data.page - 1}" ${data.page <= 1 ? "disabled" : ""}>← Назад</button><span>Страница ${data.page} / ${Math.max(1, Math.ceil(data.total / data.limit))} · Всего ${data.total}</span><button data-action="page" data-page="${data.page + 1}" ${data.page * data.limit >= data.total ? "disabled" : ""}>Далее →</button></div>`;
}
const statuses = ["Принят", "В работе", "Собран", "Готов к выдаче", "Отменён"];
const staffCancellationReasons = [
  "Не удалось связаться с клиентом",
  "Товар закончился на складе",
  "Клиент не оплатил заказ",
  "Клиент попросил отменить заказ",
];
const orderNumber = (order) => String(order.order_number).padStart(6, "0");
async function orderWorkspace(page) {
  const params = new URLSearchParams({ page: String(page), limit: "8" });
  for (const [k, v] of Object.entries(state.orderFilters))
    if (v) params.set(k, v);
  const data = await api("/order?" + params);
  const f = state.orderFilters;
  $("#workspace-content").innerHTML =
    `<form id="order-filter" class="toolbar"><label>Статус<select name="status"><option value="">Все статусы</option>${statuses.map((s) => `<option ${f.status === s ? "selected" : ""}>${s}</option>`).join("")}</select></label><label>С даты<input name="from" type="date" value="${esc(f.from?.slice(0, 10) || "")}"></label><label>По дату<input name="to" type="date" value="${esc(f.to?.slice(0, 10) || "")}"></label><button class="secondary">Применить</button><button type="button" class="text-button" data-action="reset-order-filters">Сбросить фильтры</button></form>${data.items.length ? data.items.map((o) => `<article class="order-card"><div class="order-card-head"><h3>ЗАКАЗ № ${orderNumber(o)}</h3><span class="status">${esc(o.status)}</span></div><p>${date(o.date_time)}${state.user.role === "client" ? "" : ` · ${esc(o.user_name)} · ${esc(o.user_email)}`}</p><div class="order-summary"><span>${money(o.total)}</span><button class="text-button" data-action="order" data-id="${o.id}">Открыть заказ ↗</button></div></article>`).join("") : '<p class="empty">Заказов пока нет.</p>'}${pager(data)}`;
}
async function orderModal(id) {
  const o = await api("/order/" + id);
  const staff = state.user.role !== "client";
  const canClientCancel = ["Принят", "В работе"].includes(o.status);
  const options = statuses;
  const customer = o.customer;
  const customerDetails =
    staff && customer
      ? `<details class="order-customer"><summary>Покупатель: ${esc(customer.name)}</summary><dl><div><dt>Имя</dt><dd>${esc(customer.name)}</dd></div><div><dt>Телефон</dt><dd>${customer.phone ? `<a href="tel:${esc(customer.phone)}">${esc(customer.phone)}</a>` : "Не указан"}</dd></div><div><dt>Почта</dt><dd><a href="mailto:${esc(customer.email)}">${esc(customer.email)}</a></dd></div><div><dt>Дата регистрации</dt><dd>${date(customer.registration_date)}</dd></div></dl></details>`
      : "";
  const actor = {
    client: "покупатель",
    manager: "менеджер",
    admin: "администратор",
  }[o.cancellation_actor];
  const cancellation =
    o.status === "Отменён"
      ? `<section class="cancellation-details"><h3>Причина отмены</h3><p>${esc(o.cancellation_reason || "Причина не указана: заказ отменён до обновления приложения.")}</p>${actor ? `<p class="muted">Отменил: ${actor}${o.cancelled_at ? " / " + date(o.cancelled_at) : ""}</p>` : ""}</section>`
      : "";
  const form =
    staff && o.status !== "Отменён"
      ? `<form id="status-form" data-id="${o.id}" class="form-stack"><label>Статус заказа<select name="status">${options.map((s) => `<option ${s === o.status ? "selected" : ""}>${s}</option>`).join("")}</select></label><div class="cancel-reason" hidden><label>Причина отмены<select name="cancellation_reason_choice" disabled><option value="">Выберите причину</option>${staffCancellationReasons.map((reason) => `<option value="${esc(reason)}">${esc(reason)}</option>`).join("")}<option value="custom">Другое</option></select></label><label class="cancel-custom-reason" hidden>Комментарий<textarea name="cancellation_reason" maxlength="1000" disabled></textarea></label></div><p class="form-error" role="alert"></p><button class="primary">Сохранить статус</button></form>`
      : !staff && canClientCancel
        ? `<form id="client-cancel-form" data-id="${o.id}" class="form-stack client-cancellation"><details><summary>Отменить заказ</summary><label>Причина отмены (необязательно)<textarea name="cancellation_reason" maxlength="1000"></textarea></label><button class="secondary danger" type="submit">Подтвердить отмену</button></details><p class="form-error" role="alert"></p></form>`
        : "";
  showModal(
    "Заказ № " + orderNumber(o),
    `<div class="modal-body"><p class="muted">${date(o.date_time)}${staff ? "" : `<br>${esc(o.user_name)} / ${esc(o.user_email)}`}</p>${customerDetails}<div class="status-progress">${
      o.status === "Отменён"
        ? '<p class="current-status">Заказ отменён</p>'
        : statuses
            .slice(0, 4)
            .map(
              (s, i) =>
                `<p class="${s === o.status ? "current-status" : ""}">${s === o.status ? "→ " : i < statuses.indexOf(o.status) ? "✓ " : ""}${s}</p>`,
            )
            .join("")
    }</div>${cancellation}<h3 class="order-items-title">Состав заказа</h3>${o.items.map((v) => `<article class="cart-line">${picture(v)}<div><h3>${esc(v.name)}</h3><p>${esc(v.color)} / ${esc(v.size)}</p><p>${v.count} шт. × ${money(v.price)}</p><p>${money(v.count * v.price)}</p></div></article>`).join("")}<div class="cart-total"><span>Итого</span><span>${money(o.total)}</span></div>${form}</div>`,
    "drawer",
  );
}
async function productWorkspace(page) {
  const data = await api(`/product?page=${page}&limit=12`);
  $("#workspace-content").innerHTML =
    `<div class="toolbar"><button class="secondary" data-action="new-product">Добавить товар +</button></div><div class="table-wrap"><table><thead><tr><th>Название</th><th>Размер / цвет</th><th>Цена</th><th>Действие</th></tr></thead><tbody>${data.items.map((p) => `<tr><td><a class="stock-product-link" href="/?product=${encodeURIComponent(p.id)}" data-action="inventory-product" data-id="${esc(p.id)}">${esc(p.name)}</a></td><td>${esc(p.size)} / ${esc(p.color)}</td><td>${money(p.price)}</td><td><button data-action="edit-product" data-id="${p.id}">Изменить ↗</button></td></tr>`).join("")}</tbody></table></div>${pager(data)}`;
}
async function inventoryWorkspace(page) {
  const params = new URLSearchParams({ page: String(page), limit: "12" });
  if (state.inventoryQuery) params.set("q", state.inventoryQuery);
  const data = await api("/product?" + params);
  $("#workspace-content").innerHTML =
    `<form id="inventory-search" class="toolbar"><label>Поиск по названию и бренду<input type="search" name="q" maxlength="120" value="${esc(state.inventoryQuery)}" placeholder="Например, худи" autocomplete="off"></label><button type="submit" class="secondary">Найти</button><button type="button" class="text-button" data-action="reset-inventory-search">Сбросить поиск</button></form>${!data.items.length ? '<p class="empty">Товары не найдены.</p>' : ""}<div class="table-wrap"><table><thead><tr><th>Товар</th><th>Размер / цвет</th><th>На складе, шт.</th><th></th></tr></thead><tbody>${data.items.map((p) => `<tr><td><a class="stock-product-link" href="/?product=${encodeURIComponent(p.id)}" data-action="inventory-product" data-id="${esc(p.id)}">${esc(p.name)}</a></td><td>${esc(p.size)} / ${esc(p.color)}</td><td>${state.user.role === "admin" ? `<input type="number" min="0" max="1000000" step="1" value="${p.stock}" aria-label="Остаток: ${esc(p.name)}, ${esc(p.size)}, ${esc(p.color)}" data-stock-id="${p.id}" class="stock-quantity">` : String(p.stock)}</td><td>${state.user.role === "admin" ? `<button type="button" data-action="save-stock" data-id="${p.id}" data-expected="${p.stock}">Сохранить</button>` : ""}</td></tr>`).join("")}</tbody></table></div>${pager(data)}`;
}
function creationStockFields(form) {
  const box = $("[data-creation-stock]", form);
  if (!box) return;
  const previous = new Map(
    $$("[data-stock-size]", box).map((input) => [
      JSON.stringify([input.dataset.stockSize, input.dataset.stockColor]),
      input.value,
    ]),
  );
  const sizes = $$("[name=attr_size]:checked", form).map(
    (input) => input.value,
  );
  const colors = $$("[name=attr_color]:checked", form).map(
    (input) => input.value,
  );
  box.innerHTML = `<p class="mono">Количество на складе</p>${sizes.length && colors.length ? sizes.map((size) => colors.map((color) => `<label>${esc(size)} / ${esc(color)}<input type="number" min="0" max="1000000" step="1" required value="${esc(previous.get(JSON.stringify([size, color])) ?? "0")}" data-stock-size="${esc(size)}" data-stock-color="${esc(color)}"></label>`).join("")).join("") : '<p class="field-help">Выберите размеры и цвет товара.</p>'}`;
}
function validEmail(value) {
  const email = value.trim().toLowerCase();
  const parts = email.split("@");
  if (parts.length !== 2 || email.length > 254) return false;
  const [local, domain] = parts;
  if (
    !local ||
    local.length > 64 ||
    !/^[a-z0-9.!#$%&'*+/=?^_`{|}~-]+$/.test(local) ||
    local.startsWith(".") ||
    local.endsWith(".") ||
    local.includes("..") ||
    domain.length > 253
  )
    return false;
  const labels = domain.split(".");
  return (
    labels.length >= 2 &&
    /^[a-z]{2,63}$/.test(labels.at(-1)) &&
    labels.every((label) => /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(label))
  );
}
function validateInput(input) {
  let error = "";
  if (input.name === "email" && input.value && !validEmail(input.value))
    error = "Почта должна быть в формате local@domain.tld";
  if (input.name === "phone" && input.value) {
    const compact = input.value.replace(/[ ()-]/g, "");
    if (!/^(?:\+7|8)[3489][0-9]{9}$/.test(compact))
      error = "Укажите российский номер: +7 XXX XXX-XX-XX или 8XXXXXXXXXX";
  }
  if (input.name === "password" && input.value) {
    if ([...input.value].length < 8)
      error = "Пароль должен содержать минимум 8 символов";
    else if (new TextEncoder().encode(input.value).length > 72)
      error = "Пароль слишком длинный. Используйте более короткий пароль";
  }
  if (input.name === "password_confirmation") {
    const password = input.form?.elements.namedItem("password");
    input.required = Boolean(password?.required || password?.value);
    if (input.value !== (password?.value || ""))
      error = "Пароли не совпадают";
  }
  if (
    input.name === "price" &&
    input.value &&
    (!/^[0-9]{1,7}(\.[0-9]{1,2})?$/.test(input.value) ||
      Number(input.value) > 1000000)
  )
    error = "Цена: от 0 до 1 000 000 ₽, до двух знаков после точки";
  input.setCustomValidity(error);
}
function formatPhone(value) {
  let digits = String(value).replace(/\D/g, "");
  if (!digits) return "";
  if (String(value).trim().startsWith("+") && digits[0] !== "7")
    return String(value)
      .replace(/[^0-9+ ()-]/g, "")
      .slice(0, 18);
  if (digits[0] === "7" || digits[0] === "8") digits = digits.slice(1);
  digits = digits.slice(0, 10);
  let result = "+7";
  if (digits.length) result += " (" + digits.slice(0, 3);
  if (digits.length > 3) result += ") " + digits.slice(3, 6);
  if (digits.length > 6) result += "-" + digits.slice(6, 8);
  if (digits.length > 8) result += "-" + digits.slice(8, 10);
  return result;
}
function maskPhone(input) {
  const raw = input.value;
  const cursor = input.selectionStart ?? raw.length;
  let before = raw.slice(0, cursor).replace(/\D/g, "").length;
  const digits = raw.replace(/\D/g, "");
  if (before && digits[0] !== "7" && digits[0] !== "8") before++;
  input.value = formatPhone(raw);
  let caret = 0,
    seen = 0;
  while (caret < input.value.length && seen < before) {
    if (/\d/.test(input.value[caret])) seen++;
    caret++;
  }
  input.setSelectionRange(caret, caret);
  validateInput(input);
}
document.addEventListener("beforeinput", (event) => {
  const input = event.target;
  if (
    !(input instanceof HTMLInputElement) ||
    input.name !== "phone" ||
    input.selectionStart !== input.selectionEnd
  )
    return;
  const cursor = input.selectionStart;
  const backward = event.inputType === "deleteContentBackward";
  const forward = event.inputType === "deleteContentForward";
  if (!backward && !forward) return;
  let index = backward ? cursor - 1 : cursor;
  if (index < 0 || index >= input.value.length || /\d/.test(input.value[index]))
    return;
  while (
    index >= 0 &&
    index < input.value.length &&
    !/\d/.test(input.value[index])
  )
    index += backward ? -1 : 1;
  if (index < 0 || index >= input.value.length) return;
  event.preventDefault();
  input.setRangeText(
    "",
    backward ? index : cursor,
    backward ? cursor : index + 1,
    "end",
  );
  maskPhone(input);
});
document.addEventListener("input", (event) => {
  const input = event.target;
  if (!(input instanceof HTMLInputElement)) return;
  if (input.name === "phone") maskPhone(input);
  validateInput(input);
  if (input.name === "password") {
    const confirmation = input.form?.elements.namedItem("password_confirmation");
    if (confirmation) validateInput(confirmation);
  }
});
function field(
  label,
  name,
  value = "",
  type = "text",
  required = true,
  maxLength,
) {
  if (type === "tel" && value) value = formatPhone(value);
  const limit =
    maxLength ??
    { name: 100, email: 254, phone: 18, size: 20, color: 60 }[name] ??
    120;
  let attributes = `maxlength="${limit}"`;
  if (type === "tel")
    attributes +=
      ' autocomplete="tel" inputmode="tel" placeholder="+7 (900) 000-00-00"';
  if (type === "email") attributes += ' autocomplete="email"';
  if (type === "text" && name === "name") attributes += ' autocomplete="name"';
  if (type === "number") {
    type = "text";
    attributes =
      'inputmode="decimal" maxlength="10" pattern="[0-9]{1,7}([.][0-9]{1,2})?" title="От 0 до 1 000 000 ₽, до двух знаков после точки"';
  }
  return `<label>${esc(label)}<input name="${name}" type="${type}" value="${esc(value)}" ${required ? "required" : ""} ${attributes}></label>`;
}
const attributeLabels = {
  type: "Тип",
  gender: "Пол",
  size: "Размер",
  color: "Цвет",
  brand: "Бренд",
  season: "Сезон",
};
const attributeLimits = {
  type: 120,
  gender: 120,
  size: 20,
  color: 60,
  brand: 120,
  season: 120,
};
function attributeField(key, product, options) {
  const selected = product.attributes?.[key]?.length
    ? product.attributes[key]
    : product[key]
      ? [product[key]]
      : [];
  const values = [...new Set([...(options[key] || []), ...selected])];
  const label = attributeLabels[key];
  return `<details class="product-attribute" data-attribute="${key}"><summary><span>${label}</span><span data-attribute-summary>${esc(selected.join(", ") || "Выбрать")}</span></summary><div class="attribute-panel">${key === "size" ? '<p class="field-help">Цифры — для обуви, буквы — для одежды.</p>' : ""}<div class="attribute-options">${values.map((value) => attributeChoice(key, value, selected.includes(value))).join("")}</div>${["season", "size"].includes(key) ? "" : `<div class="attribute-add"><input class="attribute-add-input" data-custom-value maxlength="${attributeLimits[key]}" placeholder="Свой вариант" aria-label="Свой вариант: ${label}"><button class="text-button" type="button" data-action="add-attribute" aria-label="Добавить значение: ${label}">Добавить</button></div>`}</div></details>`;
}
const shoeSizes = ["39", "40", "41", "42", "43", "44", "45"];
const clothingSizes = ["XS", "S", "M", "L", "XL", "XXL", "XXXL", "4XL"];
function syncProductSizes(form) {
  const types = $$("[name=attr_type]:checked", form).map(
    (input) => input.value,
  );
  const footwear = types.some((type) =>
    /обув|кроссов|кед|ботин|туфл|сапог|слипон|сандал|тапоч|мокасин|лофер|балетк|босонож|угги|сабо|сланц/i.test(
      type,
    ),
  );
  const control = $('[data-attribute="size"]', form);
  const selected = new Set(
    $$("input:checked", control).map((input) => input.value.toUpperCase()),
  );
  const sizes = types.length ? (footwear ? shoeSizes : clothingSizes) : [];
  $(".attribute-options", control).innerHTML = sizes
    .map((size) => attributeChoice("size", size, selected.has(size)))
    .join("");
  $(".field-help", control).textContent = !types.length
    ? "Сначала выберите тип товара."
    : footwear
      ? "Размеры обуви: 39–45."
      : "Размеры одежды: XS–4XL.";
  updateAttributeSummary(control);
  creationStockFields(form);
}
function attributeChoice(key, value, checked = false) {
  const label = attributeValueLabel(key, value);
  return `<label class="attribute-choice"><input type="checkbox" name="attr_${key}" value="${esc(value)}" aria-label="${esc(attributeLabels[key])}: ${esc(label)}" ${checked ? "checked" : ""}><span>${esc(label)}</span></label>`;
}
function updateAttributeSummary(control) {
  const values = $$("input[type=checkbox]:checked", control).map(
    (input) => input.value,
  );
  $("[data-attribute-summary]", control).textContent =
    values.join(", ") || "Выбрать";
}
function addAttributeValue(button) {
  const control = button.closest(".product-attribute");
  const input = $("[data-custom-value]", control);
  const value = input.value.trim();
  if (!value) {
    input.focus();
    return false;
  }
  if (/[\r\n\t]/.test(value)) {
    toast("Значение должно быть в одну строку");
    return false;
  }
  const existing = $$("input[type=checkbox]", control).find(
    (el) => el.value.toLocaleLowerCase() === value.toLocaleLowerCase(),
  );
  if (
    (!existing || !existing.checked) &&
    $$("input[type=checkbox]:checked", control).length >= 10
  ) {
    toast("Можно выбрать до 10 значений");
    return false;
  }
  if (existing) existing.checked = true;
  else
    $(".attribute-options", control).insertAdjacentHTML(
      "beforeend",
      attributeChoice(control.dataset.attribute, value, true),
    );
  input.value = "";
  updateAttributeSummary(control);
  if (control.dataset.attribute === "type")
    syncProductSizes(control.closest("form"));
  creationStockFields(control.closest("form"));
  input.focus();
  return true;
}
document.addEventListener("change", (event) => {
  const control = event.target.closest(".product-attribute");
  if (!control) return;
  if ($$("input[type=checkbox]:checked", control).length > 10) {
    event.target.checked = false;
    toast("Можно выбрать до 10 значений");
  }
  updateAttributeSummary(control);
  if (control.dataset.attribute === "type")
    syncProductSizes(control.closest("form"));
  creationStockFields(control.closest("form"));
});
document.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && event.target.matches(".attribute-add-input")) {
    event.preventDefault();
    addAttributeValue(
      $(
        "[data-action=add-attribute]",
        event.target.closest(".product-attribute"),
      ),
    );
  }
});
function clearProductPhotoQueue(form) {
  if (!form) return;
  for (const entry of productPhotoQueue.get(form) || [])
    URL.revokeObjectURL(entry.url);
  productPhotoQueue.delete(form);
}
function renderProductPhotoQueue(form) {
  const entries = productPhotoQueue.get(form) || [];
  const list = $("[data-selected-photos]", form);
  list.innerHTML = entries
    .map(
      (entry, index) =>
        `<div class="selected-photo"><img src="${entry.url}" alt="${esc(entry.file.name)}"><span>${esc(entry.file.name)}</span><button type="button" class="text-button" data-action="remove-selected-photo" data-index="${index}" aria-label="Убрать фото ${esc(entry.file.name)}">Убрать</button></div>`,
    )
    .join("");
  $("[data-photo-count]", form).textContent = entries.length
    ? `Выбрано фото: ${entries.length} из 10`
    : "";
}
document.addEventListener("change", (event) => {
  const input = event.target;
  if (!input.matches('#product-form input[type="file"]')) return;
  const form = input.form;
  const entries = productPhotoQueue.get(form) || [];
  const known = new Set(
    entries.map((entry) =>
      JSON.stringify([
        entry.file.name,
        entry.file.size,
        entry.file.lastModified,
        entry.file.type,
      ]),
    ),
  );
  const added = [...input.files].filter((file) => {
    const key = JSON.stringify([
      file.name,
      file.size,
      file.lastModified,
      file.type,
    ]);
    if (known.has(key)) return false;
    known.add(key);
    return true;
  });
  input.value = "";
  if (entries.length + added.length > 10) {
    toast("Можно добавить до 10 фотографий");
    return;
  }
  if (
    added.some(
      (file) =>
        !["image/jpeg", "image/png"].includes(file.type) ||
        file.size > 8 * 1024 * 1024,
    )
  ) {
    toast("Выберите PNG или JPEG, каждый файл до 8 МБ");
    return;
  }
  if (
    [...entries.map((entry) => entry.file), ...added].reduce(
      (sum, file) => sum + file.size,
      0,
    ) >
    30 * 1024 * 1024
  ) {
    toast("Общий размер фотографий: до 30 МБ");
    return;
  }
  productPhotoQueue.set(form, [
    ...entries,
    ...added.map((file) => ({ file, url: URL.createObjectURL(file) })),
  ]);
  renderProductPhotoQueue(form);
});
modal.addEventListener("close", () =>
  clearProductPhotoQueue($("#product-form")),
);
async function editProduct(id) {
  const options = await api("/product/options");
  const p = id
    ? await api("/product/" + id)
    : {
        name: "",
        type: "",
        gender: "Унисекс",
        size: "",
        color: "",
        description: "",
        brand: "StyleShop",
        season: "",
        price: 0,
        image_index: -1,
        photos: [],
      };
  showModal(
    id ? "Изменить товар" : "Новый товар",
    `<div class="modal-body"><form class="form-stack" id="product-form" data-id="${esc(id || "")}"><div class="form-grid">${field("Название", "name", p.name, "text", true, 120)}${Object.keys(
      attributeLabels,
    )
      .map((key) => attributeField(key, p, options))
      .join(
        "",
      )}${!id ? '<section class="full form-stack" data-creation-stock></section>' : `<p class="full">На складе: ${p.stock} шт. Изменить остаток можно в разделе «Склад».</p>`}${field("Цена в рублях", "price", p.price / 100, "number")}<label class="full">Описание<textarea name="description" maxlength="4000" required>${esc(p.description)}</textarea></label><label class="full">${id ? "Добавить фотографии" : "Фотографии товара"}<input name="file" type="file" accept="image/jpeg,image/png" multiple><span class="field-help">До 10 фото, каждое до 8 МБ, вместе до 30 МБ. Можно добавлять по одному.</span></label><div class="full" aria-live="polite"><p class="field-help" data-photo-count></p><div class="selected-photos" data-selected-photos></div></div></div><p class="form-error" role="alert"></p><button class="primary">${id ? "Сохранить товар" : "Добавить товар"}</button></form>${id ? `<details class="photo-manager"><summary>Загруженные фотографии (${p.photos.length})</summary><div class="photo-list">${p.photos.map((photo) => `<div><a href="/photo/${photo.id}" target="_blank" rel="noopener"><img src="/photo/${photo.id}" alt="Фото товара"></a><button data-action="delete-photo" data-id="${photo.id}" data-product="${id}">Удалить фото</button></div>`).join("")}</div></details><button class="text-button danger" data-action="ask-delete" data-kind="product" data-id="${id}">Удалить этот вариант товара</button>` : ""}</div>`,
  );
  syncProductSizes($("#product-form"));
  creationStockFields($("#product-form"));
}
async function userWorkspace(page) {
  const data = await api(`/user?page=${page}&limit=12`);
  $("#workspace-content").innerHTML =
    `<div class="toolbar"><p class="mono muted">Управление аккаунтами и доступом</p><button class="secondary" data-action="new-user">Добавить пользователя +</button></div><div class="table-wrap"><table><thead><tr><th>Имя</th><th>Email</th><th>Роль</th><th>Регистрация</th><th>Действие</th></tr></thead><tbody>${data.items.map((u) => `<tr><td>${esc(u.name)}</td><td>${esc(u.email)}</td><td>${esc(u.role)}</td><td>${date(u.registration_date)}</td><td><button data-action="edit-user" data-id="${u.id}">Изменить ↗</button></td></tr>`).join("")}</tbody></table></div>${pager(data)}`;
  state.users = data.items;
}
async function editUser(id) {
  state.roles = await api("/role");
  const u = id
    ? state.users.find((v) => v.id === id)
    : {
        name: "",
        email: "",
        phone: "",
        role_id: state.roles.find((v) => v.name === "client").id,
      };
  if (!u) return;
  showModal(
    id ? "Пользователь" : "Новый пользователь",
    `<div class="modal-body"><form class="form-stack" id="user-form" data-id="${esc(id || "")}">${field("Имя", "name", u.name)}${field("Email", "email", u.email, "email")}${field("Телефон", "phone", u.phone, "tel", true)}<label>Роль<select name="role_id">${state.roles.map((v) => `<option value="${v.id}" ${v.id === u.role_id ? "selected" : ""}>${esc(v.name)}</option>`).join("")}</select></label><label>${id ? "Новый пароль (необязательно)" : "Пароль"}<input name="password" type="password" minlength="8" maxlength="72" autocomplete="new-password" ${id ? "" : "required"}></label><p class="field-help">Минимум 8 символов.</p><label>Повторите пароль<input name="password_confirmation" type="password" minlength="8" maxlength="72" autocomplete="new-password" ${id ? "" : "required"}></label><p class="form-error" role="alert"></p><button class="primary">Сохранить</button></form>${id && id !== state.user.id ? `<button class="text-button danger" data-action="ask-delete" data-kind="user" data-id="${id}">Удалить пользователя</button>` : ""}</div>`,
  );
}
async function roleWorkspace() {
  state.roles = await api("/role");
  $("#workspace-content").innerHTML =
    `<div class="toolbar"><p class="mono muted">Системные роли защищены от удаления</p><button class="secondary" data-action="new-role">Добавить роль +</button></div><div class="table-wrap"><table><thead><tr><th>Роль</th><th>ID</th><th>Действие</th></tr></thead><tbody>${state.roles.map((r) => `<tr><td>${esc(r.name)}</td><td class="mono">${esc(r.id)}</td><td>${["admin", "manager", "client"].includes(r.name) ? '<span class="muted">Системная</span>' : `<button data-action="edit-role" data-id="${r.id}">Изменить ↗</button>`}</td></tr>`).join("")}</tbody></table></div><p class="cart-note">Дополнительные роли не получают привилегии администратора, менеджера или покупателя автоматически.</p>`;
}
function editRole(id) {
  const r = id ? state.roles.find((v) => v.id === id) : { name: "" };
  showModal(
    id ? "Изменить роль" : "Новая роль",
    `<div class="modal-body"><form class="form-stack" id="role-form" data-id="${esc(id || "")}">${field("Название роли", "name", r.name, "text", true, 50)}<p class="form-error" role="alert"></p><button class="primary">Сохранить</button></form>${id ? `<button class="text-button danger" data-action="ask-delete" data-kind="role" data-id="${id}">Удалить роль</button>` : ""}</div>`,
  );
}
const profileFields = {
  name: { label: "Имя", type: "text", saved: "Имя изменено" },
  phone: { label: "Телефон", type: "tel", saved: "Телефон изменён" },
  email: { label: "Почта", type: "email", saved: "Почта изменена" },
  password: { label: "Пароль", type: "password" },
};
function profileRow(key) {
  const config = profileFields[key];
  const value =
    key === "password" ? "••••••••" : state.user[key] || "Не указан";
  return `<section class="profile-row" data-profile-field="${key}"><div class="profile-row-summary"><div><h3>${config.label}</h3><p>${esc(value)}</p></div><button type="button" class="text-button" data-action="profile-edit" data-field="${key}" aria-label="Изменить ${key === "email" ? "почту" : config.label.toLowerCase()}" aria-controls="profile-editor-${key}" aria-expanded="false">Изменить</button></div><div id="profile-editor-${key}" hidden></div></section>`;
}
function profileWorkspace() {
  $("#workspace-content").innerHTML =
    `<div class="profile-content"><p class="mono muted">${esc(roleLabel(state.user.role))}</p><div class="profile-fields">${Object.keys(profileFields).map(profileRow).join("")}</div><div class="profile-actions"><button class="text-button" data-action="logout">Выйти из аккаунта ↗</button></div></div>`;
}
function editProfileField(button) {
  const key = button.dataset.field;
  const config = profileFields[key];
  if (!config) return;
  const editor = $("#profile-editor-" + key);
  editor.innerHTML = `<form class="form-stack profile-field-form" data-field="${key}">${key === "password" ? '<label>Новый пароль<input name="password" type="password" minlength="8" maxlength="72" autocomplete="new-password" required></label><p class="field-help">Минимум 8 символов.</p><label>Повторите пароль<input name="password_confirmation" type="password" minlength="8" maxlength="72" autocomplete="new-password" required></label>' : field(config.label, key, state.user[key], config.type)}<p class="form-error" role="alert"></p><div class="profile-edit-actions"><button type="submit" class="secondary">Сохранить</button><button type="button" class="text-button" data-action="profile-cancel" data-field="${key}">Отмена</button></div></form>`;
  editor.hidden = false;
  button.hidden = true;
  button.setAttribute("aria-expanded", "true");
  $("input", editor).focus();
}
function cancelProfileField(button) {
  const key = button.dataset.field;
  const editor = $("#profile-editor-" + key);
  editor.innerHTML = "";
  editor.hidden = true;
  const edit = $(`[data-action="profile-edit"][data-field="${key}"]`);
  edit.hidden = false;
  edit.setAttribute("aria-expanded", "false");
  edit.focus();
}
async function catalogView() {
  $("#store-view").hidden = false;
  $("#workspace-view").hidden = true;
  await loadProducts();
}
const actions = {
  close: () => modal.close(),
  login: () => authModal(),
  register: () => authModal(true),
  privacy: showPrivacy,
  "add-attribute": addAttributeValue,
  "profile-edit": editProfileField,
  "profile-cancel": cancelProfileField,
  "privacy-close": () => $("#privacy-modal").close(),
  catalog: async () => {
    if (modal.open) modal.close();
    await catalogView();
    $("#catalog").scrollIntoView({
      behavior: matchMedia("(prefers-reduced-motion: reduce)").matches
        ? "instant"
        : "smooth",
    });
  },
  account: () => workspace("profile"),
  workspace: (b) => workspace(b.dataset.tab),
  page: (b) => workspace(state.tab, Number(b.dataset.page)),
  product: (b) => productModal(Number(b.dataset.index)),
  "gallery-select": (b) => changeGallery(Number(b.dataset.index)),
  "gallery-prev": () => changeGallery(state.selected.galleryIndex - 1),
  "gallery-next": () => changeGallery(state.selected.galleryIndex + 1),
  "gallery-enlarge": enlargeGallery,
  "lightbox-close": () => lightbox.close(),
  size: async (b) => {
    const selected = state.selected;
    selected.id = b.dataset.id;
    for (const button of $$(".size", modal))
      button.setAttribute("aria-pressed", String(button === b));
    const p = await api("/product/" + b.dataset.id);
    if (state.selected !== selected || selected.id !== p.id || !modal.open)
      return;
    selected.product = p;
    syncPurchaseQuantity(false);
    selected.galleryIndex = 0;
    $("#detail-price").textContent = money(p.price);
    $("#detail-stock").textContent = `На складе: ${p.stock} шт.`;
    $(".product-gallery", modal).outerHTML = galleryMarkup(p);
    $(".description", modal).textContent = p.description;
  },
  "add-cart": async () => {
    if (!state.user) {
      authModal();
      toast("Войди, чтобы добавить вещь в корзину");
      return;
    }
    if (state.user.role !== "client") {
      toast("Корзина доступна только покупателям");
      return;
    }
    const quantity = $("#purchase-quantity");
    if (quantity.disabled) { toast("Нет доступного количества этого размера"); return; }
    if (!quantity.reportValidity()) return;
    const selected = state.selected;
    await api("/cart", {method:"POST",body:{product_id:selected.id,count:Number(quantity.value)}});
    await loadCart();
    if (state.selected===selected) syncPurchaseQuantity();
    toast(`Размер ${selected.product.size} добавлен в корзину`);
  },
  cart: async () => {
    await loadCart();
    cartModal();
  },
  "cart-count": async (b) => {
    const n = Number(b.dataset.count);
    await api("/cart/" + b.dataset.id, {
      method: n === 0 ? "DELETE" : "PUT",
      ...(n === 0 ? {} : { body: { count: n } }),
    });
    await loadCart();
    cartModal();
  },
  "cart-delete": async (b) => {
    await api("/cart/" + b.dataset.id, { method: "DELETE" });
    await loadCart();
    cartModal();
  },
  checkout: async () => {
    const cart_ids = chosenCartItems().map((item) => item.id);
    if (!cart_ids.length) {
      toast("Выбери хотя бы один товар");
      return;
    }
    const o = await api("/order", { method: "POST", body: { cart_ids } });
    await loadCart();
    await workspace("orders");
    await orderModal(o.id);
    toast("Заказ принят");
  },
  "inventory-product": (button) => productById(button.dataset.id),
  "reset-inventory-search": async () => {
    state.inventoryQuery = "";
    await workspace("inventory", 1);
  },
  "remove-selected-photo": (button) => {
    const form = button.closest("form"),
      entries = productPhotoQueue.get(form) || [];
    const [removed] = entries.splice(Number(button.dataset.index), 1);
    if (removed) URL.revokeObjectURL(removed.url);
    productPhotoQueue.set(form, entries);
    renderProductPhotoQueue(form);
  },
  "save-stock": async (button) => {
    const input = $(`[data-stock-id="${button.dataset.id}"]`);
    if (!input.reportValidity()) return;
    await api(`/product/${button.dataset.id}/stock`, {
      method: "PATCH",
      body: {
        stock: Number(input.value),
        expected_stock: Number(button.dataset.expected),
      },
    });
    await workspace("inventory", state.page);
    toast("Остаток сохранён");
  },
  order: (b) => orderModal(b.dataset.id),
  "reset-order-filters": async () => {
    state.orderFilters = {};
    await workspace("orders", 1);
  },
  "clear-filter": async (button) => {
    clearTimeout(filterTimer);
    for (const input of $$(
      `input[name="${button.dataset.key}"]`,
      $("#filters"),
    ))
      input.checked = false;
    updateFilterSummaries();
    await loadProducts();
  },
  "reset-filters": async () => {
    clearTimeout(filterTimer);
    $("#filters").reset();
    for (const control of $$("#filters input, #filters select")) {
      if (control.type === "checkbox") control.checked = false;
      else control.value = "";
    }
    for (const dropdown of $$(".filter-dropdown")) dropdown.open = false;
    updateFilterSummaries();
    await loadProducts();
  },
  "new-product": () => editProduct(),
  "edit-product": (b) => editProduct(b.dataset.id),
  "new-user": () => editUser(),
  "edit-user": (b) => editUser(b.dataset.id),
  "new-role": () => editRole(),
  "edit-role": (b) => editRole(b.dataset.id),
  "delete-photo": async (b) => {
    await api("/photo/" + b.dataset.id, { method: "DELETE" });
    await editProduct(b.dataset.product);
    toast("Фото удалено");
  },
  "ask-delete": (b) => {
    if ($(".confirm-line", modal)) return;
    const box = document.createElement("div");
    box.className = "confirm-line";
    box.innerHTML = `<p>Подтвердить удаление?</p><button class="text-button danger" data-action="confirm-delete" data-kind="${esc(b.dataset.kind)}" data-id="${esc(b.dataset.id)}">Да, удалить</button><button class="text-button" data-action="cancel-delete">Отмена</button>`;
    b.after(box);
  },
  "cancel-delete": (b) => b.closest(".confirm-line").remove(),
  "confirm-delete": async (b) => {
    await api("/" + b.dataset.kind + "/" + b.dataset.id, { method: "DELETE" });
    modal.close();
    await workspace(state.tab, state.page);
    state.catalogOptions = null;
    await loadProducts();
    toast("Запись удалена");
  },
  logout: async () => {
    await api("/auth/logout", { method: "POST", body: {} });
    state.user = null;
    await loadCart();
    await catalogView();
    if (modal.open) modal.close();
    toast("Ты вышел из аккаунта");
  },
};
document.addEventListener("click", async (e) => {
  const b = e.target.closest("[data-action]");
  if (!b || b.disabled) return;
  if (b.tagName === "A") {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
  }
  const action = actions[b.dataset.action];
  if (!action) return;
  b.disabled = true;
  try {
    await action(b);
  } catch (err) {
    toast(err.message);
  } finally {
    b.disabled = b.dataset.action === "add-cart" ? Boolean($("#purchase-quantity")?.disabled) : false;
  }
});
document.addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = e.target;
  $$("input", form).forEach(validateInput);
  if (!form.reportValidity()) return;
  const submit = $("[type=submit], button:not([type])", form);
  const error = $(".form-error", form);
  if (error) error.textContent = "";
  if (submit) submit.disabled = true;
  const v = Object.fromEntries(new FormData(form));
  const id = form.dataset.id;
  try {
    switch (form.id) {
      case "auth-form": {
        if (form.dataset.register === "true") {
          v.privacy_accepted = form.elements.privacy_accepted.checked;
          await api("/user", { method: "POST", body: v });
        }
        const login = await api("/auth/login", {
          method: "POST",
          body: { email: v.email, password: v.password },
        });
        state.user = login.user;
        await loadCart();
        modal.close();
        toast("Вход выполнен");
        if (state.user.role !== "client") await workspace("orders");
        break;
      }
      case "inventory-search":
        state.inventoryQuery = String(v.q || "").trim();
        await workspace("inventory", 1);
        break;
      case "order-filter": {
        state.orderFilters = {
          status: v.status,
          from: v.from ? v.from + "T00:00:00Z" : "",
          to: v.to ? v.to + "T23:59:59Z" : "",
        };
        await workspace("orders");
        break;
      }
      case "client-cancel-form":
        await api("/order/" + id, {
          method: "PUT",
          body: {
            status: "Отменён",
            cancellation_reason: v.cancellation_reason,
          },
        });
        await workspace("orders", state.page);
        await orderModal(id);
        toast("Заказ отменён");
        break;
      case "status-form": {
        const body = { status: v.status };
        if (v.status === "Отменён") {
          body.cancellation_reason =
            v.cancellation_reason_choice === "custom"
              ? v.cancellation_reason.trim()
              : v.cancellation_reason_choice;
          if (!body.cancellation_reason)
            throw new Error("Укажите причину отмены заказа");
        }
        await api("/order/" + id, { method: "PUT", body });
        await workspace("orders", state.page);
        await orderModal(id);
        toast("Статус сохранён");
        break;
      }
      case "product-form": {
        const attributes = {};
        for (const key of Object.keys(attributeLabels)) {
          const control = $(`[data-attribute="${key}"]`, form);
          const pending = $("[data-custom-value]", control)?.value.trim() || "";
          if (
            pending &&
            !addAttributeValue($("[data-action=add-attribute]", control))
          )
            throw new Error("Проверьте новое значение характеристики");
          attributes[key] = $$("input[type=checkbox]:checked", control).map(
            (input) => input.value,
          );
          if (!attributes[key].length)
            throw new Error(`Выберите значение: ${attributeLabels[key]}`);
          v[key] = attributes[key][0];
        }
        if (attributes.size.length * attributes.color.length > 25)
          throw new Error(
            "Можно сохранить до 25 сочетаний размеров и цветов за раз",
          );
        for (const key of Object.keys(v))
          if (key.startsWith("attr_")) delete v[key];
        v.attributes = attributes;
        if (!id) {
          v.stock_by_variant = {};
          for (const input of $$("[data-stock-size]", form)) {
            (v.stock_by_variant[input.dataset.stockSize] ??= {})[
              input.dataset.stockColor
            ] = Number(input.value);
          }
        }
        const files = (productPhotoQueue.get(form) || []).map(
          (entry) => entry.file,
        );
        if (
          files.length > 10 ||
          files.some((file) => file.size > 8 * 1024 * 1024)
        )
          throw new Error("Выберите до 10 фото, каждое не больше 8 МБ");
        delete v.file;
        v.price = Math.round(Number(v.price) * 100);
        if (!id) v.image_index = -1;
        const data = new FormData();
        data.append("product", JSON.stringify(v));
        for (const file of files) data.append("file", file);
        const p = await api("/product" + (id ? "/" + id : ""), {
          method: id ? "PUT" : "POST",
          form: data,
        });
        await workspace("products", state.page);
        state.catalogOptions = null;
        await loadProducts();
        await editProduct(p.id);
        toast(
          p.variant_ids?.length > 1
            ? `Сохранено вариантов: ${p.variant_ids.length}`
            : id
              ? "Товар сохранён"
              : "Товар добавлен",
        );
        break;
      }
      case "user-form":
        await api("/user" + (id ? "/" + id : ""), {
          method: id ? "PUT" : "POST",
          body: v,
        });
        modal.close();
        await workspace("users", state.page);
        toast("Пользователь сохранён");
        break;
      case "role-form":
        await api("/role" + (id ? "/" + id : ""), {
          method: id ? "PUT" : "POST",
          body: v,
        });
        modal.close();
        await workspace("roles");
        toast("Роль сохранена");
        break;
      default: {
        if (!form.matches(".profile-field-form")) break;
        const key = form.dataset.field;
        if (!profileFields[key]) break;
        if (key === "password" && v.password !== v.password_confirmation)
          throw new Error("Пароли не совпадают");
        const updated = await api("/auth/me", {
          method: "PATCH",
          body: key === "password" ? { password: v.password, password_confirmation: v.password_confirmation } : { [key]: v[key] },
        });
        if (key === "password") {
          state.user = null;
          await loadCart();
          await catalogView();
          authModal();
          toast("Пароль изменён. Войди снова");
        } else {
          state.user = updated;
          accountNav();
          const row = form.closest(".profile-row");
          if (row?.isConnected) {
            row.outerHTML = profileRow(key);
            $(`[data-action="profile-edit"][data-field="${key}"]`).focus();
          }
          toast(profileFields[key].saved);
        }
        break;
      }
    }
  } catch (err) {
    if (error && error.isConnected) error.textContent = err.message;
    else toast(err.message);
  } finally {
    if (submit) submit.disabled = false;
  }
});
let filterTimer;
$("#filters").addEventListener("input", (e) => {
  if (e.target.tagName !== "INPUT" || e.target.type === "checkbox") return;
  clearTimeout(filterTimer);
  filterTimer = setTimeout(
    () => loadProducts().catch((e) => toast(e.message)),
    250,
  );
});
$("#filters").addEventListener("change", (e) => {
  updateFilterSummaries();
  if (e.target.tagName === "SELECT" || e.target.type === "checkbox")
    loadProducts().catch((e) => toast(e.message));
});
$("#filters").addEventListener("submit", (e) => {
  e.preventDefault();
  loadProducts().catch((e) => toast(e.message));
});
modal.addEventListener("click", (e) => {
  if (e.target === modal) {
    const r = modal.getBoundingClientRect();
    if (
      e.clientX < r.left ||
      e.clientX > r.right ||
      e.clientY < r.top ||
      e.clientY > r.bottom
    )
      modal.close();
  }
});
async function init() {
  try {
    state.user = await api("/auth/me");
  } catch (err) {
    if (err.status !== 401) toast(err.message);
  }
  accountNav();
  await Promise.all([loadProducts(), loadCart()]);
  const productId = new URLSearchParams(location.search).get("product");
  if (productId) await productById(productId);
}
init().catch((err) => {
  $("#products").innerHTML =
    `<div class="empty">${errorMarkup(err.message)}<button class="text-button" data-action="reset-filters">Попробовать снова</button></div>`;
});
