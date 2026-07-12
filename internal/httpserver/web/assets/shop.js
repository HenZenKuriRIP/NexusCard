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
 const m = {
 apple_id: '\uF8FF', apple_gc: '\uD83C\uDF81', google: 'G', netflix: 'N',
 streaming: '\u25B6', data: '\u2197', other: '\u2726',
 };
 // Prefer emoji where available
 const e = { apple_id: '🍎', apple_gc: '🎁', google: '🔵', netflix: '🎬', streaming: '🎧', data: '📡', other: '📦' };
 return e[code] || m[code] || '📦';
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
 document.title = (__cfg.title || 'NexusCard') + ' · ProductsStore';
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
 <small>Digital Goods Store</small>
 </span>
 </a>
 <nav class="nx-nav">
 <a class="nav-link ${!activeCat ? 'on' : ''}" href="#/">All</a>
 ${navCats}
 </nav>
 </div>
 </header>
 ${inner}
 <footer class="nx-foot">
 <div class="nx-foot-inner">
 <div>
 <b>${esc(__cfg.site_name || __cfg.title || 'NexusCard')}</b>
 <p>Digital goods auto-delivery after payment</p>
 </div>
 <div class="nx-foot-meta">
 <span>Auto ship</span><span>Orderslookup</span><span>Alipay channel</span>
 </div>
 </div>
 <div class="nx-copy">Demo digital goods. Comply with local laws and terms.</div>
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
 const chips = [{ code: '', name: 'All' }].concat(__cats).map(c =>
 `<button class="chip ${(!cat && !c.code) || cat === c.code ? 'on' : ''}" data-c="${esc(c.code || '')}">${esc(c.name)}</button>`
 ).join('');

 const cards = (products || []).map(p => productCard(p)).join('') ||
 `<div class="empty-box">No products in this category yet.</div>`;

 await shell(`
 <section class="nx-hero">
 <div class="nx-hero-inner">
 <div class="nx-hero-copy">
 <div class="pill-row">
 <span class="pill">Auto delivery</span>
 <span class="pill soft">US zone</span>
 <span class="pill soft">Streaming</span>
 </div>
 <h1>${esc(__cfg.title || 'ProductsStore')}</h1>
 <p>${esc(__cfg.subtitle || '')}</p>
 <ul class="hero-points">${feats}</ul>
 <div class="hero-cta">
 <a class="btn primary lg" href="#grid">Browse Products</a>
 <a class="btn outline lg" href="#/buy/1" id="hotLink">Popular Apple ID</a>
 </div>
 </div>
 <div class="nx-hero-art">
 <div class="float-card c1"><span>🍎</span><div><b>Apple ID US</b><small>Ready · instant</small></div></div>
 <div class="float-card c2"><span>🎁</span><div><b>App Store cards</b><small>$10 / $50</small></div></div>
 <div class="float-card c3"><span>🎬</span><div><b>Netflix / Google</b><small>Accounts</small></div></div>
 <div class="float-card c4"><span>📡</span><div><b>DataData</b><small>Auto delivery</small></div></div>
 </div>
 </div>
 </section>

 <section class="nx-section" id="grid">
 <div class="section-head">
 <div>
 <h2>Browse products</h2>
 <p class="muted">Filter by category · secrets after payment</p>
 </div>
 </div>
 <div class="chip-row" id="chips">${chips}</div>
 <div class="product-grid">${cards}</div>
 </section>

 <section class="nx-section dim">
 <div class="trust-grid">
 <div class="trust"><b>01</b><h3>Pay instantly</h3><p>AlipayCheckout，Secure checkout。</p></div>
 <div class="trust"><b>02</b><h3>Auto delivery</h3><p>Secrets shown after payment.</p></div>
 <div class="trust"><b>03</b><h3>Categories</h3><p>Apple, Google, Netflix, streaming and data.</p></div>
 <div class="trust"><b>04</b><h3>Track orders</h3><p>Keep the cashier link to reopen delivery.</p></div>
 </div>
 </section>
 `, cat);

 // hot link to first apple product if any
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
 ? '<span class="stock ok">In stock</span>'
 : '<span class="stock no">Out of stock</span>';
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
 ? `<a class="btn primary" href="#/buy/${p.id}">Buy</a>`
 : `<button class="btn" disabled>Restocking</button>`}
 </div>
 </div>
 </article>`;
}

function catName(code) {
 const c = __cats.find(x => x.code === code);
 return c ? c.name : 'Products';
}

async function buyPage(id) {
 const p = await api('/products/' + id);
 const feats = (p.features || []).map(f => `<li>${esc(f)}</li>`).join('') || '<li>Auto delivery after payment</li>';
 await shell(`
 <section class="nx-section narrow">
 <a class="back" href="#/">← Back to store</a>
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
 <div class="field"><label>Email (optional)</label>
 <input id="email" type="email" placeholder="you@example.com"/>
 </div>
 <div id="err" class="alert err hidden"></div>
 <button class="btn primary lg block" id="pay" ${p.in_stock ? '' : 'disabled'}>
 ${p.in_stock ? 'Buy now' : 'Out of stock'}
 </button>
 <p class="fineprint">Secrets appear on the cashier page after payment. Save them yourself.Digital goods are non-refundable once delivered.</p>
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
 <a class="back" href="#/">← Back to store</a>
 <div class="order-card">
 <div class="order-head">
 <h1>Order details</h1>
 <span class="badge ${esc(o.status)}">${esc(o.status)}</span>
 </div>
 <p class="muted">${esc(o.subject)} · <code>${esc(o.out_trade_no)}</code></p>
 <div class="price xl">${yuan(o.amount)}</div>
 ${paid
 ? `<div class="deliver-wrap">
 <h3>Delivery (copy and save now)</h3>
 <div class="deliver-box" id="box">${esc(o.delivered || 'Paid，Generating…')}</div>
 <button class="btn outline" id="copy">Copy secret</button>
 </div>`
 : o.status === 'pending'
 ? `<a class="btn primary" href="${esc(o.cashier_url)}">Continue payment</a>`
 : `<p class="muted">Order closed or expired</p>`}
 </div>
 </section>
 `);
 const copy = $('#copy');
 if (copy) {
 copy.onclick = async () => {
 try {
 await navigator.clipboard.writeText($('#box').textContent);
 copy.textContent = 'Copied';
 } catch {
 alert('Copy manually');
 }
 };
 }
}

boot();
