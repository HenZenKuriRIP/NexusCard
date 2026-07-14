const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];

function yuan(c) {
  return '¥' + (Number(c || 0) / 100).toFixed(2);
}
function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
}
function catIcon(code) {
  const e = { apple_id: '🍎', apple_gc: '🎁', google: '🔵', netflix: '🎬', streaming: '🎧', data: '📡', other: '📦' };
  return e[code] || '📦';
}
function statusLabel(status) {
  const m = {
    pending: '待支付', paid: '已支付', closed: '已关闭', expired: '已过期', paid_orphan: '异常已付',
  };
  return m[status] || status || '-';
}

async function api(path, opts) {
  const res = await fetch('/api/shop/v1' + path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function route() {
  const h = (location.hash || '#/').replace(/^#\/?/, '');
  const [name, id] = h.split('/').filter(Boolean);
  return { name: name || 'home', id, cat: new URLSearchParams(location.hash.split('?')[1] || '').get('c') || '' };
}

let __cfg = null;
let __cats = [];

async function boot() {
  document.body.className = 'shop';
  window.addEventListener('hashchange', render);
  try {
    __cfg = await api('/config');
    const cr = await api('/categories');
    __cats = cr.categories || [];
  } catch (e) {
    __cfg = { title: 'NexusCard', subtitle: '' };
  }
  document.title = (__cfg.title || 'NexusCard') + ' · 数字商品商城';
  await render();
}

function shell(inner, activeCat) {
  const navCats = __cats.slice(0, 6).map(c =>
    `<a class="nav-link ${activeCat === c.code ? 'on' : ''}" href="#/?c=${esc(c.code)}">${esc(c.name)}</a>`
  ).join('');
  $('#app').innerHTML = `
 <header class="nx-top">
 <div class="nx-top-inner">
 <a class="nx-logo" href="#/">
 <span class="nx-mark">N</span>
 <span>
 <b>${esc(__cfg.title || 'NexusCard')}</b>
 <small>数字商品商城</small>
 </span>
 </a>
 <nav class="nx-nav">
 <a class="nav-link ${!activeCat ? 'on' : ''}" href="#/">全部</a>
 ${navCats}
 </nav>
 </div>
 </header>
 ${inner}
 <footer class="nx-foot">
 <div class="nx-foot-inner">
 <div>
 <b>${esc(__cfg.site_name || __cfg.title || 'NexusCard')}</b>
 <p>支付成功后自动发货</p>
 </div>
 <div class="nx-foot-meta">
 <span>自动发货</span><span>订单可查</span><span>安全支付</span>
 </div>
 </div>
 <div class="nx-copy">数字商品一经发货概不退款，请遵守当地法律法规与平台条款。</div>
 </footer>`;
}

async function render() {
  const r = route();
  if (r.name === 'buy') return buyPage(r.id);
  if (r.name === 'order') return orderPage(r.id);
  return homePage(r.cat);
}

async function homePage(cat) {
  const q = cat ? '?category=' + encodeURIComponent(cat) : '';
  const { products } = await api('/products' + q);
  const feats = (__cfg.features || []).map(f => `<li>${esc(f)}</li>`).join('');
  const chips = [{ code: '', name: '全部' }].concat(__cats).map(c =>
    `<button class="chip ${(!cat && !c.code) || cat === c.code ? 'on' : ''}" data-c="${esc(c.code || '')}">${esc(c.name)}</button>`
  ).join('');

  const cards = (products || []).map(p => productCard(p)).join('') ||
    `<div class="empty-box">该分类暂无商品</div>`;

  await shell(`
 <section class="nx-hero">
 <div class="nx-hero-inner">
 <div class="nx-hero-copy">
 <div class="pill-row">
 <span class="pill">自动发货</span>
 <span class="pill soft">美区账号</span>
 <span class="pill soft">流媒体</span>
 </div>
 <h1>${esc(__cfg.title || '数字商品商城')}</h1>
 <p>${esc(__cfg.subtitle || '')}</p>
 <ul class="hero-points">${feats}</ul>
 <div class="hero-cta">
 <a class="btn primary lg" href="#grid">浏览商品</a>
 <a class="btn outline lg" href="#/buy/1" id="hotLink">热门 Apple ID</a>
 </div>
 </div>
 <div class="nx-hero-art">
 <div class="float-card c1"><span>🍎</span><div><b>美区 Apple ID</b><small>即开即用</small></div></div>
 <div class="float-card c2"><span>🎁</span><div><b>App Store 卡</b><small>$10 / $50</small></div></div>
 <div class="float-card c3"><span>🎬</span><div><b>Netflix / Google</b><small>账号类</small></div></div>
 <div class="float-card c4"><span>📡</span><div><b>流量套餐</b><small>自动发货</small></div></div>
 </div>
 </div>
 </section>

 <section class="nx-section" id="grid">
 <div class="section-head">
 <div>
 <h2>商品列表</h2>
 <p class="muted">按分类筛选 · 支付后显示卡密</p>
 </div>
 </div>
 <div class="chip-row" id="chips">${chips}</div>
 <div class="product-grid">${cards}</div>
 </section>

 <section class="nx-section dim">
 <div class="trust-grid">
 <div class="trust"><b>01</b><h3>即时支付</h3><p>支持支付宝 / 易支付，安全收银台。</p></div>
 <div class="trust"><b>02</b><h3>自动发货</h3><p>付款成功后立即展示卡密信息。</p></div>
 <div class="trust"><b>03</b><h3>丰富品类</h3><p>Apple、Google、Netflix、流媒体与流量。</p></div>
 <div class="trust"><b>04</b><h3>订单可查</h3><p>保留收银台链接，随时查看发货内容。</p></div>
 </div>
 </section>
 `, cat);

  const apple = (products || []).find(p => p.category === 'apple_id');
  if (apple) {
    const a = $('#hotLink');
    if (a) a.href = '#/buy/' + apple.id;
  }
  $$('#chips .chip').forEach(b => {
    b.onclick = () => {
      const c = b.dataset.c;
      location.hash = c ? '#/?c=' + c : '#/';
    };
  });
}

function productCard(p) {
  const stock = p.in_stock
    ? '<span class="stock ok">有货</span>'
    : '<span class="stock no">缺货</span>';
  const badge = p.badge ? `<span class="badge-tag">${esc(p.badge)}</span>` : '';
  const region = p.region ? `<span class="region">${esc(p.region)}</span>` : '';
  return `
 <article class="p-card cat-${esc(p.category || 'other')}">
 <div class="p-cover">
 ${badge}
 <div class="p-icon">${esc(p.icon || catIcon(p.category))}</div>
 <div class="p-cover-meta">${region}</div>
 </div>
 <div class="p-body">
 <div class="p-cat">${esc(catName(p.category))}</div>
 <h3>${esc(p.name)}</h3>
 <p class="p-desc">${esc(p.description || '')}</p>
 <div class="p-foot">
 <div>
 <div class="price">${yuan(p.price_cents)}</div>
 ${stock}
 </div>
 ${p.in_stock
    ? `<a class="btn primary" href="#/buy/${p.id}">购买</a>`
    : `<button class="btn" disabled>补货中</button>`}
 </div>
 </div>
 </article>`;
}

function catName(code) {
  const c = __cats.find(x => x.code === code);
  return c ? c.name : '商品';
}

async function buyPage(id) {
  const p = await api('/products/' + id);
  const feats = (p.features || []).map(f => `<li>${esc(f)}</li>`).join('') || '<li>支付成功后自动发货</li>';
  await shell(`
 <section class="nx-section narrow">
 <a class="back" href="#/">← 返回商城</a>
 <div class="buy-layout">
 <div class="buy-visual cat-${esc(p.category || 'other')}">
 <div class="p-icon xl">${esc(p.icon || catIcon(p.category))}</div>
 <div class="buy-tags">
 ${p.badge ? `<span class="badge-tag">${esc(p.badge)}</span>` : ''}
 ${p.region ? `<span class="region">${esc(p.region)}</span>` : ''}
 </div>
 </div>
 <div class="buy-panel">
 <div class="p-cat">${esc(catName(p.category))}</div>
 <h1>${esc(p.name)}</h1>
 <p class="lead">${esc(p.description || '')}</p>
 <div class="price xl">${yuan(p.price_cents)} <small>${esc(p.currency || 'CNY')}</small></div>
 <ul class="feat-list">${feats}</ul>
 <div class="field"><label>邮箱（可选）</label>
 <input id="email" type="email" placeholder="you@example.com"/>
 </div>
 <div id="err" class="alert err hidden"></div>
 <button class="btn primary lg block" id="pay" ${p.in_stock ? '' : 'disabled'}>
 ${p.in_stock ? '立即购买' : '暂时缺货'}
 </button>
 <p class="fineprint">卡密将在收银台支付成功后展示，请自行妥善保存。数字商品发货后不支持退款。</p>
 </div>
 </div>
 </section>
 `, p.category);

  const btn = $('#pay');
  if (btn && !btn.disabled) {
    btn.onclick = async () => {
      $('#err').classList.add('hidden');
      btn.disabled = true;
      try {
        const r = await api('/checkout', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ product_id: Number(id), email: $('#email').value.trim() }),
        });
        location.href = r.cashier_url;
      } catch (e) {
        $('#err').textContent = e.message;
        $('#err').classList.remove('hidden');
        btn.disabled = false;
      }
    };
  }
}

async function orderPage(token) {
  const o = await api('/orders/by-token?token=' + encodeURIComponent(token));
  const paid = o.status === 'paid' || o.status === 'paid_orphan';
  await shell(`
 <section class="nx-section narrow">
 <a class="back" href="#/">← 返回商城</a>
 <div class="order-card">
 <div class="order-head">
 <h1>订单详情</h1>
 <span class="badge ${esc(o.status)}">${esc(statusLabel(o.status))}</span>
 </div>
 <p class="muted">${esc(o.subject)} · <code>${esc(o.out_trade_no)}</code></p>
 <div class="price xl">${yuan(o.amount)}</div>
 ${paid
    ? `<div class="deliver-wrap">
 <h3>发货内容（请立即复制保存）</h3>
 <div class="deliver-box" id="box">${esc(o.delivered || '已支付，正在生成…')}</div>
 <button class="btn outline" id="copy">复制卡密</button>
 </div>`
    : o.status === 'pending'
      ? `<a class="btn primary" href="${esc(o.cashier_url)}">继续支付</a>`
      : `<p class="muted">订单已关闭或已过期</p>`}
 </div>
 </section>
 `);
  const copy = $('#copy');
  if (copy) {
    copy.onclick = async () => {
      try {
        await navigator.clipboard.writeText($('#box').textContent);
        copy.textContent = '已复制';
      } catch {
        alert('请手动复制');
      }
    };
  }
}

boot();
