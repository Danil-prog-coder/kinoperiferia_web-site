/* Кинопериферия — прогрессивное улучшение.
   Без JS сайт полностью работоспособен: фильтры и сортировка — обычные ссылки
   и форма с submit, карточка изделия — отдельная страница. Этот файл добавляет
   фильтрацию без перезагрузки, быстрый просмотр в оверлее, параллакс и появление
   блоков при прокрутке. */
(function () {
  'use strict';

  var reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  /* ── Сортировка: отправляем форму сразу при выборе ────────────────── */
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
        // Меняем блок целиком: вместе с сеткой обновляются переключатель
        // «Посмотреть все» и число скрытых карточек.
        var fresh = doc.querySelector('[data-catalog-body]');
        var current = document.querySelector('[data-catalog-body]');
        if (!fresh || !current) throw new Error('нет сетки каталога');
        current.replaceWith(fresh);
        grid = fresh.querySelector('#catalog-grid');

        var counter = doc.querySelector('#catalog .eyebrow');
        var liveCounter = document.querySelector('#catalog .eyebrow');
        if (counter && liveCounter) liveCounter.textContent = counter.textContent;

        if (push) history.pushState({ catalog: url }, '', url);
      });
  }

  filters.forEach(function (link) {
    link.addEventListener('click', function (e) {
      if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
      if (!('fetch' in window) || !('DOMParser' in window)) return;
      e.preventDefault();

      filters.forEach(function (other) { other.removeAttribute('aria-current'); });
      link.setAttribute('aria-current', 'true');

      var hidden = document.querySelector('.sort input[name="cat"]');
      var slug = link.getAttribute('data-filter');
      if (hidden) {
        if (slug === 'all') hidden.remove();
        else hidden.value = slug;
      } else if (slug !== 'all') {
        var form = document.querySelector('[data-catalog-form]');
        if (form) {
          var input = document.createElement('input');
          input.type = 'hidden';
          input.name = 'cat';
          input.value = slug;
          form.querySelector('.sort').appendChild(input);
        }
      }

      swapCatalog(link.href, true).catch(function () {
        window.location.href = link.href;
      });
    });
  });

  /* ── Быстрый просмотр изделия ─────────────────────────────────────── */
  var overlay = document.getElementById('quick-view');
  var lastFocus = null;
  var quickViewOpen = false;

  /* Фотография карточки и фотография в быстром просмотре получают на время
     перехода одно имя — браузер сам переводит одну в другую. Имя должно быть
     в документе ровно одно, поэтому у карточки его забираем в тот же момент,
     когда отдаём оверлею. */
  var MEDIA_NAME = 'kp-media';
  var activeMedia = null;

  function morphSupported() {
    return !reduced &&
      typeof document.startViewTransition === 'function' &&
      document.visibilityState === 'visible';
  }

  // Заглушка повторяет интерфейс startViewTransition: код вызова один и тот же
  // и там, где переходов нет.
  function morph(update) {
    if (!morphSupported()) {
      update();
      return { finished: Promise.resolve(), updateCallbackDone: Promise.resolve() };
    }

    var transition = document.startViewTransition(update);

    // Пока вкладка не перерисовывается, браузер откладывает переход, а вместе
    // с ним и саму подмену разметки. Страховка: если за треть секунды кадр не
    // случился, показываем результат без анимации.
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

  function moneyRow(p) {
    var old = p.oldPriceLabel ? '<span class="product__old">' + esc(p.oldPriceLabel) + '</span>' : '';
    return '<div class="product__price-row"><span class="product__price">' +
      esc(p.priceLabel) + '</span>' + old + '</div>';
  }

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  /* Фотографии у части изделий ещё нет — вместо неё та же плашка, что и на
     витрине, чтобы быстрый просмотр не открывался с битой картинкой. */
  function media(p) {
    if (p.hasPhoto) {
      return '<img class="product__img" src="' + esc(p.image) + '" alt="' + esc(p.alt) + '">';
    }
    return '<div class="ph" role="img" aria-label="Фотография изделия «' + esc(p.name) + '» появится позже">' +
      '<span class="ph__mark">КП</span><span class="ph__text">Фото скоро</span></div>';
  }

  function variantsTable(p) {
    var variants = p.variants || [];
    if (!variants.length) return '';

    var head = (p.optionNames || []).map(function (name) {
      return '<th scope="col">' + esc(name) + '</th>';
    }).join('') + '<th scope="col" class="variants__price">Цена</th>';

    var rows = variants.map(function (v) {
      var cells = (v.values || []).map(function (value) {
        return '<td>' + esc(value) + '</td>';
      }).join('');
      var cls = 'variants__price' + (v.price ? '' : ' variants__price--unknown');
      return '<tr>' + cells + '<td class="' + cls + '">' + esc(v.priceLabel) + '</td></tr>';
    }).join('');

    return '<table class="variants">' +
      '<caption class="variants__caption">Исполнения и цены</caption>' +
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
          moneyRow(p) +
          variantsTable(p) +
          '<dl class="specs">' + specs + '</dl>' +
          '<div class="product__actions">' +
            '<a class="btn btn--solid" href="' + esc(p.telegram) + '" target="_blank" rel="noopener noreferrer">Уточнить наличие и заказать ↗</a>' +
            '<a class="btn btn--ghost" href="/order?product=' + encodeURIComponent(p.slug) + '">Оставить заявку</a>' +
            '<a class="btn btn--ghost" href="/product/' + encodeURIComponent(p.slug) + '">Открыть страницу</a>' +
          '</div>' +
        '</div>' +
      '</div>';
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
          overlay.classList.remove('overlay--closing');
          overlay.scrollTop = 0;
          document.body.style.overflow = 'hidden';
        });

        history.pushState({ quickView: slug }, '', href);

        // Фокус переносим после того, как разметка оверлея оказалась в DOM.
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
    overlay.classList.remove('overlay--closing');
  }

  function closeQuickView(pop) {
    // Флаг снимаем сразу: скрытие идёт по таймеру ради анимации, а popstate
    // прилетает раньше — без флага оверлей закрывался бы дважды.
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
      overlay.classList.add('overlay--closing');
      window.setTimeout(hideOverlay, reduced ? 0 : 250);
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
      // Возврат к предыдущему фильтру: у первой записи истории state пустой,
      // поэтому ориентируемся на текущий адрес, а не на сохранённое состояние.
      if (document.getElementById('catalog-grid')) {
        swapCatalog(window.location.href, false).catch(function () {
          window.location.reload();
        });
      }
    });
  }

  /* ── Параллакс главного изображения ───────────────────────────────── */
  var hero = document.getElementById('hero-img');
  if (hero && !reduced) {
    var pointerY = 0;
    var smoothed = 0;
    var frame = 0;

    window.addEventListener('pointermove', function (e) {
      pointerY = (e.clientY / window.innerHeight) * 2 - 1;
    }, { passive: true });

    var step = function () {
      frame = requestAnimationFrame(step);
      smoothed += (pointerY - smoothed) * 0.06;
      var box = hero.parentNode.getBoundingClientRect();
      if (box.bottom < 0 || box.top > window.innerHeight) return;
      var out = Math.max(-1, Math.min(1, (window.innerHeight / 2 - (box.top + box.height / 2)) / window.innerHeight));
      hero.style.transform = 'translate3d(0,' + (smoothed * -10 + out * 26) + 'px,0) scale(' + (1.06 + Math.abs(out) * 0.04) + ')';
    };
    frame = requestAnimationFrame(step);
  }

  /* ── Появление блоков при прокрутке ───────────────────────────────── */
  var revealables = Array.prototype.slice.call(document.querySelectorAll('[data-reveal]'));
  if (revealables.length && !reduced && 'IntersectionObserver' in window) {
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        entry.target.classList.remove('is-hidden');
        io.unobserve(entry.target);
      });
    }, { threshold: 0.1 });

    revealables.forEach(function (node) {
      if (node.getBoundingClientRect().top > window.innerHeight * 0.92) {
        node.classList.add('is-hidden');
      }
      io.observe(node);
    });
  }
})();
