/* Кинопериферия — прогрессивное улучшение.
   Без JS сайт полностью работоспособен: фильтры каталога — обычные ссылки,
   форма заявки — обычный submit, карточка изделия — отдельная страница.
   Этот файл добавляет фильтрацию без перезагрузки, быстрый просмотр в
   боковой панели, шапку со scroll-spy и мобильное меню. Раздел 7 ТЗ убрал
   декоративную анимацию: здесь нет ни появления блоков при скролле, ни
   параллакса — только отклик на действия пользователя. */
(function () {
  'use strict';

  var reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  /* ── Корзина заявки ────────────────────────────────────────────────
     Живёт в localStorage браузера — на сайте нет аккаунтов и оплаты,
     корзина лишь собирает список для одной заявки перед отправкой в
     /order. Каждая позиция — изделие, выбранное исполнение (строкой,
     если было) и количество. */
  var CART_KEY = 'kp:cart';

  function cartRead() {
    try {
      var raw = window.localStorage.getItem(CART_KEY);
      var list = raw ? JSON.parse(raw) : [];
      return Array.isArray(list) ? list : [];
    } catch (e) {
      return [];
    }
  }

  function cartWrite(list) {
    try { window.localStorage.setItem(CART_KEY, JSON.stringify(list)); } catch (e) { /* приватный режим — корзина проживёт только вкладку */ }
    cartSync();
  }

  function cartClear() {
    try { window.localStorage.removeItem(CART_KEY); } catch (e) {}
    cartSync();
  }

  function cartLineKey(item) { return item.slug + '|' + (item.variant || ''); }

  function cartAdd(item) {
    var list = cartRead();
    var key = cartLineKey(item);
    var existing = list.filter(function (x) { return cartLineKey(x) === key; })[0];
    if (existing) {
      existing.qty = Math.min(50, (existing.qty || 1) + item.qty);
    } else {
      list.push(item);
    }
    cartWrite(list);
  }

  function cartRemove(index) {
    var list = cartRead();
    list.splice(index, 1);
    cartWrite(list);
  }

  function cartSetQty(index, qty) {
    var list = cartRead();
    if (!list[index]) return;
    list[index].qty = Math.max(1, Math.min(50, Math.round(qty) || 1));
    cartWrite(list);
  }

  function cartCount() {
    return cartRead().reduce(function (sum, x) { return sum + (x.qty || 1); }, 0);
  }

  function formatRub(v) {
    var s = String(Math.round(v));
    var out = '';
    for (var i = 0; i < s.length; i++) {
      if (i > 0 && (s.length - i) % 3 === 0) out += ' ';
      out += s[i];
    }
    return out + ' ₽';
  }

  function cartSync() {
    var count = cartCount();
    Array.prototype.slice.call(document.querySelectorAll('[data-cart-count]')).forEach(function (el) {
      el.textContent = String(count);
      el.hidden = count === 0;
    });
    renderCartOnOrderPage();
  }

  /* Список корзины на странице /order: рисуется только там, где есть
     контейнер data-cart-list. Пока корзина пуста, остаётся обычный
     select одиночного изделия — без JS это единственный путь заказа. */
  function renderCartOnOrderPage() {
    var list = document.querySelector('[data-cart-list]');
    if (!list) return;
    var items = cartRead();
    var itemsInput = document.querySelector('[data-cart-items-input]');
    var productField = document.querySelector('[data-product-field]');

    if (!items.length) {
      list.hidden = true;
      list.innerHTML = '';
      if (itemsInput) itemsInput.value = '';
      if (productField) productField.hidden = false;
      return;
    }

    if (productField) productField.hidden = true;
    list.hidden = false;

    var rows = items.map(function (item) {
      var unit = typeof item.price === 'number' ? item.price : 0;
      var lineLabel = unit ? formatRub(unit * item.qty) : (item.priceLabel || 'XXX');
      var media = item.image
        ? '<img class="cart-item__img" src="' + esc(item.image) + '" alt="">'
        : '<span class="cart-item__ph" aria-hidden="true">КП</span>';
      return (
        '<li class="cart-item">' +
          '<div class="cart-item__media">' + media + '</div>' +
          '<div class="cart-item__body">' +
            '<span class="cart-item__name">' + esc(item.name) + '</span>' +
            (item.variant ? '<span class="cart-item__variant">' + esc(item.variant) + '</span>' : '') +
            '<div class="cart-item__row">' +
              '<div class="qty" data-qty>' +
                '<button type="button" data-qty-dec aria-label="Меньше">−</button>' +
                '<span class="qty__value">' + item.qty + '</span>' +
                '<button type="button" data-qty-inc aria-label="Больше">+</button>' +
              '</div>' +
              '<span class="cart-item__price mono">' + esc(lineLabel) + '</span>' +
              '<button type="button" class="cart-item__remove" data-cart-remove aria-label="Убрать из заявки">×</button>' +
            '</div>' +
          '</div>' +
        '</li>'
      );
    }).join('');

    var knownTotal = items.reduce(function (sum, item) { return sum + (typeof item.price === 'number' ? item.price : 0) * item.qty; }, 0);
    var hasUnknown = items.some(function (item) { return !item.price; });
    var totalLabel = (hasUnknown ? 'от ' : '') + formatRub(knownTotal);

    list.innerHTML =
      '<p class="label">Корзина заявки</p>' +
      '<ul class="cart-item-list">' + rows + '</ul>' +
      '<div class="cart-total"><span>Итого</span><span class="mono">' + totalLabel + '</span></div>';

    list.querySelectorAll('[data-qty-inc]').forEach(function (btn, i) {
      btn.addEventListener('click', function () { cartSetQty(i, items[i].qty + 1); });
    });
    list.querySelectorAll('[data-qty-dec]').forEach(function (btn, i) {
      btn.addEventListener('click', function () {
        if (items[i].qty <= 1) cartRemove(i); else cartSetQty(i, items[i].qty - 1);
      });
    });
    list.querySelectorAll('[data-cart-remove]').forEach(function (btn, i) {
      btn.addEventListener('click', function () { cartRemove(i); });
    });

    if (itemsInput) {
      itemsInput.value = JSON.stringify(items.map(function (item) {
        return { slug: item.slug, variant: item.variant || '', qty: item.qty };
      }));
    }
  }

  /* Заявка ушла успешно — очищаем корзину, чтобы её не отправили повторно. */
  if (document.querySelector('[data-order-submitted]')) cartClear();

  cartSync();

  /* ── Шапка: фон и линия при скролле ────────────────────────────────── */
  var header = document.querySelector('[data-header]');
  if (header) {
    var syncHeader = function () {
      if (window.scrollY > 4) header.setAttribute('data-scrolled', '');
      else header.removeAttribute('data-scrolled');
    };
    syncHeader();
    window.addEventListener('scroll', syncHeader, { passive: true });
  }

  /* ── Мобильное меню ────────────────────────────────────────────────── */
  var menuToggle = document.querySelector('[data-menu-toggle]');
  var mobileMenu = document.querySelector('[data-mobile-menu]');
  if (menuToggle && mobileMenu) {
    var closeMenu = function () {
      mobileMenu.hidden = true;
      menuToggle.setAttribute('aria-expanded', 'false');
      document.body.style.overflow = '';
    };
    menuToggle.addEventListener('click', function () {
      var open = mobileMenu.hidden;
      mobileMenu.hidden = !open;
      menuToggle.setAttribute('aria-expanded', String(open));
      document.body.style.overflow = open ? 'hidden' : '';
    });
    mobileMenu.addEventListener('click', function (e) {
      if (e.target.closest('a')) closeMenu();
    });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && !mobileMenu.hidden) closeMenu();
    });
  }

  /* ── Scroll-spy: подсветка активного пункта меню ──────────────────── */
  var spyNav = document.querySelector('[data-scroll-spy]');
  if (spyNav && 'IntersectionObserver' in window) {
    var spyLinks = Array.prototype.slice.call(spyNav.querySelectorAll('a[href^="/#"], a[href^="#"]'));
    var sections = spyLinks
      .map(function (a) {
        var id = a.getAttribute('href').split('#')[1];
        return { link: a, section: id ? document.getElementById(id) : null };
      })
      .filter(function (x) { return x.section; });

    if (sections.length) {
      var spy = new IntersectionObserver(function (entries) {
        entries.forEach(function (entry) {
          var match = sections.filter(function (x) { return x.section === entry.target; })[0];
          if (!match) return;
          if (entry.isIntersecting) {
            spyLinks.forEach(function (a) { a.classList.remove('is-active'); });
            match.link.classList.add('is-active');
          }
        });
      }, { rootMargin: '-40% 0px -55% 0px' });

      sections.forEach(function (x) { spy.observe(x.section); });
    }
  }

  /* ── Сортировка: отправляем форму сразу при выборе (JSON API/легаси) ─ */
  var sortSelect = document.getElementById('sort');
  if (sortSelect && sortSelect.form) {
    sortSelect.addEventListener('change', function () {
      sortSelect.form.submit();
    });
  }

  /* ── Фильтр по категориям без перезагрузки ────────────────────────── */
  var grid = document.getElementById('catalog-grid');
  var filters = Array.prototype.slice.call(document.querySelectorAll('[data-filter]'));

  function swapCatalog(url, push) {
    return fetch(url, { headers: { 'X-Requested-With': 'fetch' } })
      .then(function (r) {
        if (!r.ok) throw new Error('HTTP ' + r.status);
        return r.text();
      })
      .then(function (html) {
        var doc = new DOMParser().parseFromString(html, 'text/html');
        var fresh = doc.querySelector('[data-catalog-body]');
        var current = document.querySelector('[data-catalog-body]');
        if (!fresh || !current) throw new Error('нет сетки каталога');
        current.replaceWith(fresh);
        grid = fresh.querySelector('#catalog-grid');

        var freshTabs = doc.querySelector('[data-catalog-tabs]');
        var liveTabs = document.querySelector('[data-catalog-tabs]');
        if (freshTabs && liveTabs) liveTabs.replaceWith(freshTabs);

        var counter = doc.querySelector('#catalog .section__note');
        var liveCounter = document.querySelector('#catalog .section__note');
        if (counter && liveCounter) liveCounter.textContent = counter.textContent;

        if (push) history.pushState({ catalog: url }, '', url);
        bindFilters();
      });
  }

  function bindFilters() {
    filters = Array.prototype.slice.call(document.querySelectorAll('[data-filter]'));
    filters.forEach(function (link) {
      link.addEventListener('click', function (e) {
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
        if (!('fetch' in window) || !('DOMParser' in window)) return;
        e.preventDefault();
        swapCatalog(link.href, true).catch(function () {
          window.location.href = link.href;
        });
      });
    });
  }
  bindFilters();

  /* ── Быстрый просмотр изделия (боковая панель) ────────────────────── */
  var overlay = document.getElementById('quick-view');
  var lastFocus = null;
  var quickViewOpen = false;

  /* Фотография карточки и фотография в быстром просмотре получают на время
     перехода одно имя — браузер сам переводит одну в другую. */
  var MEDIA_NAME = 'kp-media';
  var activeMedia = null;

  function morphSupported() {
    return !reduced &&
      typeof document.startViewTransition === 'function' &&
      document.visibilityState === 'visible';
  }

  function morph(update) {
    if (!morphSupported()) {
      update();
      return { finished: Promise.resolve(), updateCallbackDone: Promise.resolve() };
    }
    var transition = document.startViewTransition(update);
    var guard = window.setTimeout(function () {
      if (typeof transition.skipTransition === 'function') transition.skipTransition();
    }, 350);
    var clearGuard = function () { window.clearTimeout(guard); };
    transition.updateCallbackDone.then(clearGuard, clearGuard);
    return transition;
  }

  function releaseMedia() {
    if (activeMedia) activeMedia.style.viewTransitionName = '';
    activeMedia = null;
  }

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  function media(p) {
    if (p.hasPhoto) {
      return '<img class="product__img" src="' + esc(p.image) + '" alt="' + esc(p.alt) + '">';
    }
    return '<div class="ph" role="img" aria-label="Фотография изделия «' + esc(p.name) + '» появится позже">' +
      '<span class="ph__mark">КП</span><span class="ph__text">Фото скоро</span></div>';
  }

  /* Фото для текущего выбора: если у исполнения есть своя фотография —
     показываем её, иначе — фото изделия по умолчанию (или плашку). */
  function mediaFor(p, variant) {
    if (variant && variant.hasPhoto) {
      return { hasPhoto: true, image: variant.image, alt: variant.alt || p.name, name: p.name };
    }
    return p;
  }

  /* Кнопка «В корзину» и счётчик количества — общая логика для быстрого
     просмотра и страницы изделия. getSelectedVariant читает текущий выбор
     исполнения на момент клика (может меняться после привязки). */
  function bindAddToCart(scope, p, getSelectedVariant) {
    var valueEl = scope.querySelector('[data-qty-value]');
    var decBtn = scope.querySelector('[data-qty-dec]');
    var incBtn = scope.querySelector('[data-qty-inc]');
    var addBtn = scope.querySelector('[data-add-to-cart]');
    var qty = 1;

    function syncQty() { if (valueEl) valueEl.textContent = String(qty); }
    if (decBtn) decBtn.addEventListener('click', function () { qty = Math.max(1, qty - 1); syncQty(); });
    if (incBtn) incBtn.addEventListener('click', function () { qty = Math.min(50, qty + 1); syncQty(); });

    if (addBtn) {
      addBtn.addEventListener('click', function () {
        var variant = getSelectedVariant ? getSelectedVariant() : null;
        var chosenMedia = mediaFor(p, variant);
        cartAdd({
          slug: p.slug,
          name: p.name,
          variant: variant ? variant.values.join(' / ') : '',
          qty: qty,
          price: variant ? variant.price : (p.price || 0),
          priceLabel: variant ? variant.priceLabel : p.priceLabel,
          image: chosenMedia.hasPhoto ? chosenMedia.image : '',
          alt: chosenMedia.hasPhoto ? chosenMedia.alt : ''
        });
        var original = addBtn.textContent;
        addBtn.textContent = 'Добавлено ✓';
        addBtn.disabled = true;
        window.setTimeout(function () {
          addBtn.textContent = original;
          addBtn.disabled = false;
        }, 1200);
      });
    }
  }

  function variantsTable(p) {
    var variants = p.variants || [];
    if (!variants.length) return '';

    var head = (p.optionNames || []).map(function (name) {
      return '<th scope="col">' + esc(name) + '</th>';
    }).join('') + '<th scope="col" class="variants__price">Цена</th>';

    var rows = variants.map(function (v, i) {
      var cells = (v.values || []).map(function (value) {
        return '<td>' + esc(value) + '</td>';
      }).join('');
      var cls = 'variants__price' + (v.price ? '' : ' variants__price--unknown');
      return '<tr data-variant="' + i + '">' + cells + '<td class="' + cls + '">' + esc(v.priceLabel) + '</td></tr>';
    }).join('');

    return '<table class="variants">' +
      '<caption class="variants__caption">Исполнения и цены — нажмите строку, чтобы выбрать</caption>' +
      '<thead><tr>' + head + '</tr></thead>' +
      '<tbody>' + rows + '</tbody></table>';
  }

  function renderOverlay(p) {
    var specs = (p.specs || []).map(function (s) {
      return '<div><dt>' + esc(s.name) + '</dt><dd>' + esc(s.value) + '</dd></div>';
    }).join('');

    var panel = overlay.querySelector('[data-qv-panel]');
    panel.innerHTML =
      '<div class="product">' +
        '<figure class="product__figure">' +
          '<div class="product__media" data-media>' + media(p) + '</div>' +
          '<figcaption class="product__caption">' + esc(p.art) + ' · ' + esc(p.categoryTitle) + '</figcaption>' +
        '</figure>' +
        '<div>' +
          '<div class="eyebrow product__tag">' + esc(p.tag) + '</div>' +
          '<h2 class="product__name">' + esc(p.name) + '</h2>' +
          '<p class="product__desc">' + esc(p.detail) + '</p>' +
          '<div class="product__price-row"><span class="product__price" data-price>' + esc(p.priceLabel) +
            '</span>' + (p.oldPriceLabel ? '<span class="product__old">' + esc(p.oldPriceLabel) + '</span>' : '') + '</div>' +
          variantsTable(p) +
          '<dl class="specs">' + specs + '</dl>' +
          '<div class="product__cart-row">' +
            '<div class="qty" data-qty>' +
              '<button type="button" data-qty-dec aria-label="Меньше">−</button>' +
              '<span class="qty__value" data-qty-value>1</span>' +
              '<button type="button" data-qty-inc aria-label="Больше">+</button>' +
            '</div>' +
            '<button class="btn btn--dark" type="button" data-add-to-cart>В корзину</button>' +
          '</div>' +
          '<div class="product__actions">' +
            '<a class="btn btn--ghost" href="/order?product=' + encodeURIComponent(p.slug) + '">Оставить заявку</a>' +
            '<a class="btn btn--ghost" href="/product/' + encodeURIComponent(p.slug) + '">Открыть страницу</a>' +
          '</div>' +
        '</div>' +
      '</div>';

    var priceEl = panel.querySelector('[data-price]');
    var mediaEl = panel.querySelector('[data-media]');
    var selectedVariant = null;
    panel.querySelectorAll('.variants tbody tr').forEach(function (row) {
      row.addEventListener('click', function () {
        var idx = Number(row.getAttribute('data-variant'));
        var variant = (p.variants || [])[idx];
        if (!variant) return;
        selectedVariant = variant;
        panel.querySelectorAll('.variants tbody tr').forEach(function (r) { r.classList.remove('is-selected'); });
        row.classList.add('is-selected');
        if (priceEl) priceEl.textContent = variant.priceLabel;
        if (mediaEl) mediaEl.innerHTML = media(mediaFor(p, variant));
      });
    });

    bindAddToCart(panel, p, function () { return selectedVariant; });
  }

  function openQuickView(card, slug, href) {
    return fetch('/api/products/' + encodeURIComponent(slug), { headers: { Accept: 'application/json' } })
      .then(function (r) {
        if (!r.ok) throw new Error('HTTP ' + r.status);
        return r.json();
      })
      .then(function (p) {
        lastFocus = document.activeElement;

        releaseMedia();
        if (morphSupported() && card) {
          activeMedia = card.querySelector('.card__media');
          if (activeMedia) activeMedia.style.viewTransitionName = MEDIA_NAME;
        }

        var transition = morph(function () {
          if (activeMedia) activeMedia.style.viewTransitionName = '';
          renderOverlay(p);
          var figure = overlay.querySelector('.product__figure');
          if (figure && activeMedia) figure.style.viewTransitionName = MEDIA_NAME;
          quickViewOpen = true;
          overlay.hidden = false;
          overlay.removeAttribute('aria-hidden');
          var scroller = overlay.querySelector('[data-qv-panel]');
          if (scroller) scroller.scrollTop = 0;
          document.body.style.overflow = 'hidden';
        });

        history.pushState({ quickView: slug }, '', href);

        return transition.updateCallbackDone.then(function () {
          var close = overlay.querySelector('.qv__close');
          if (close) close.focus();
        });
      });
  }

  function hideOverlay() {
    overlay.hidden = true;
    overlay.setAttribute('aria-hidden', 'true');
    var panel = overlay.querySelector('[data-qv-panel]');
    if (panel) panel.innerHTML = '';
  }

  function closeQuickView(pop) {
    if (!quickViewOpen) return;
    quickViewOpen = false;
    document.body.style.overflow = '';

    if (morphSupported() && activeMedia) {
      var figure = overlay.querySelector('.product__figure');
      var transition = morph(function () {
        if (figure) figure.style.viewTransitionName = '';
        if (activeMedia) activeMedia.style.viewTransitionName = MEDIA_NAME;
        hideOverlay();
      });
      transition.finished.then(releaseMedia, releaseMedia);
    } else {
      hideOverlay();
      releaseMedia();
    }

    if (!pop) history.back();
    if (lastFocus && lastFocus.focus) lastFocus.focus();
  }

  if (overlay) {
    document.addEventListener('click', function (e) {
      var closer = e.target.closest ? e.target.closest('[data-close]') : null;
      if (closer) {
        e.preventDefault();
        closeQuickView(false);
        return;
      }

      var card = e.target.closest ? e.target.closest('[data-quick-view]') : null;
      if (!card) return;
      if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
      if (!('fetch' in window)) return;
      e.preventDefault();
      openQuickView(card, card.getAttribute('data-quick-view'), card.href).catch(function () {
        window.location.href = card.href;
      });
    });

    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && !overlay.hidden) closeQuickView(false);
    });

    window.addEventListener('popstate', function () {
      if (quickViewOpen) {
        closeQuickView(true);
        return;
      }
      if (document.getElementById('catalog-grid')) {
        swapCatalog(window.location.href, false).catch(function () {
          window.location.reload();
        });
      }
    });
  }

  /* ── Страница изделия: смена фото и цены по исполнению ────────────── */
  var productArticle = document.querySelector('[data-product-page]');
  if (productArticle && 'fetch' in window) {
    var productSlug = productArticle.getAttribute('data-product-page');
    fetch('/api/products/' + encodeURIComponent(productSlug), { headers: { Accept: 'application/json' } })
      .then(function (r) {
        if (!r.ok) throw new Error('HTTP ' + r.status);
        return r.json();
      })
      .then(function (p) {
        var priceEl = productArticle.querySelector('[data-price]');
        var mediaEl = productArticle.querySelector('[data-media]');
        var selectedVariant = null;
        productArticle.querySelectorAll('.variants tbody tr[data-variant]').forEach(function (row) {
          row.addEventListener('click', function () {
            var idx = Number(row.getAttribute('data-variant'));
            var variant = (p.variants || [])[idx];
            if (!variant) return;
            selectedVariant = variant;
            productArticle.querySelectorAll('.variants tbody tr').forEach(function (r) { r.classList.remove('is-selected'); });
            row.classList.add('is-selected');
            if (priceEl) priceEl.textContent = variant.priceLabel;
            if (mediaEl) mediaEl.innerHTML = media(mediaFor(p, variant));
          });
        });
        bindAddToCart(productArticle, p, function () { return selectedVariant; });
      })
      .catch(function () { /* без JS/API таблица исполнений остаётся статичной — цену и фото не переключить, но страница рабочая */ });
  }

  /* ── Форма заявки: состояние отправки ─────────────────────────────── */
  var orderForm = document.querySelector('[data-order-form]');
  if (orderForm) {
    orderForm.addEventListener('submit', function () {
      var btn = orderForm.querySelector('[data-submit-btn]');
      if (!btn) return;
      btn.disabled = true;
      var busy = btn.getAttribute('data-busy-text');
      if (busy) btn.textContent = busy;
    });
  }
})();
