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

  /* Текст сообщения в Telegram: название, артикул и, если выбрано —
     параметры исполнения. Одна и та же логика для дефолтной ссылки и для
     клика по строке таблицы исполнений. */
  function telegramHref(p, variant) {
    var text = 'Здравствуйте! Интересует ' + p.name + ' (' + p.art + ').';
    if (variant && p.optionNames && p.optionNames.length) {
      text += ' ' + p.optionNames.join('/') + ': ' + variant.values.join('/') + '.';
    }
    return p.telegram + '?text=' + encodeURIComponent(text);
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

    overlay.innerHTML =
      '<div class="bar">' +
        '<span class="bar__brand">Кино<b>периферия</b></span>' +
        '<button class="bar__close" type="button" data-close>← В каталог</button>' +
      '</div>' +
      '<div class="rule-thin bar__rule"></div>' +
      '<div class="product">' +
        '<figure class="product__figure">' +
          media(p) +
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
          '<div class="product__actions">' +
            '<a class="btn btn--solid" data-telegram-cta href="' + esc(telegramHref(p)) + '" target="_blank" rel="noopener noreferrer">Уточнить наличие и заказать ↗</a>' +
            '<a class="btn btn--ghost" href="/order?product=' + encodeURIComponent(p.slug) + '">Оставить заявку</a>' +
            '<a class="btn btn--ghost" href="/product/' + encodeURIComponent(p.slug) + '">Открыть страницу</a>' +
          '</div>' +
        '</div>' +
      '</div>';

    var cta = overlay.querySelector('[data-telegram-cta]');
    var priceEl = overlay.querySelector('[data-price]');
    overlay.querySelectorAll('.variants tbody tr').forEach(function (row) {
      row.addEventListener('click', function () {
        var idx = Number(row.getAttribute('data-variant'));
        var variant = (p.variants || [])[idx];
        if (!variant) return;
        overlay.querySelectorAll('.variants tbody tr').forEach(function (r) { r.classList.remove('is-selected'); });
        row.classList.add('is-selected');
        if (cta) cta.href = telegramHref(p, variant);
        if (priceEl) priceEl.textContent = variant.priceLabel;
      });
    });
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
          overlay.scrollTop = 0;
          document.body.style.overflow = 'hidden';
        });

        history.pushState({ quickView: slug }, '', href);

        return transition.updateCallbackDone.then(function () {
          var close = overlay.querySelector('[data-close]');
          if (close) close.focus();
        });
      });
  }

  function hideOverlay() {
    overlay.hidden = true;
    overlay.setAttribute('aria-hidden', 'true');
    overlay.innerHTML = '';
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
