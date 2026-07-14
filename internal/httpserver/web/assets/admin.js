const API = '/admin/api/v1';
const tokenKey = 'gc_admin_token';

const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];

function token() { return localStorage.getItem(tokenKey) || ''; }
function setToken(t) { t ? localStorage.setItem(tokenKey, t) : localStorage.removeItem(tokenKey); }

async function api(path, opts = {}) {
  const headers = Object.assign({ 'Content-Type': 'application/json' }, opts.headers || {});
  if (token()) headers.Authorization = 'Bearer ' + token();
  const res = await fetch(API + path, { ...opts, headers });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { error: text }; }
  if (!res.ok) throw new Error((data && (data.error || data.message)) || res.statusText);
  return data;
}

function yuan(cents) {
  const n = Number(cents || 0);
  return '¥' + (n / 100).toFixed(2);
}
function badge(status) {
  return `<span class="badge ${status || ''}">${status || '-'}</span>`;
}
function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

const routes = {
  login: renderLogin,
  dashboard: renderDashboard,
  products: renderProducts,
  product: renderProductEdit,
  orders: renderOrders,
  merchants: renderMerchants,
  settings: renderSettings,
  payment: renderPayment,
};

function parseHash() {
  const raw = location.hash || '#/dashboard';
  const q = raw.indexOf('?');
  const path = (q >= 0 ? raw.slice(0, q) : raw).replace(/^#\/?/, '');
  const [name, id] = path.split('/');
  return { name: name || 'dashboard', id };
}

async function boot() {
  document.body.className = 'admin';
  const { name } = parseHash();
  if (name !== 'login' && !token()) {
    location.hash = '#/login';
  }
  window.addEventListener('hashchange', () => app());
  await app();
}

async function app() {
  const route = parseHash();
  if (route.name === 'login') {
    $('#app').innerHTML = '';
    await renderLogin();
    return;
  }
  if (!token()) {
    location.hash = '#/login';
    return;
  }
  let me = { username: 'admin' };
  try {
    const r = await api('/auth/me');
    me = r.user || me;
    window.__site = r.site_name || 'GiftCard';
  } catch {
    setToken('');
    location.hash = '#/login';
    return;
  }
  $('#app').innerHTML = `
    <div class="admin-layout">
      <aside class="sidebar">
        <div class="brand">
          <div class="brand-mark">G</div>
          <div>
            <h1>${esc(window.__site || 'GiftCard')}</h1>
            <p>Admin</p>
          </div>
        </div>
        <nav class="nav" id="nav">
          <a href="#/dashboard" data-r="dashboard">Dashboard</a>
          <a href="#/products" data-r="products">Products</a>
          <a href="#/orders" data-r="orders">Orders</a>
          <a href="#/merchants" data-r="merchants">Merchants / K2</a>
          <a href="#/settings" data-r="settings">Settings</a>
          <a href="#/payment" data-r="payment">Payment</a>
          <a href="/shop/" target="_blank">Open shop ↗</a>
        </nav>
      </aside>
      <section class="main">
        <div class="topbar">
          <h2 id="page-title">—</h2>
          <div class="user">${esc(me.display_name || me.username)} · <a href="#" id="logout">Sign out</a></div>
        </div>
        <div id="view"></div>
      </section>
    </div>`;
  $('#logout').onclick = (e) => { e.preventDefault(); setToken(''); location.hash = '#/login'; };
  $$('#nav a[data-r]').forEach(a => {
    if (a.dataset.r === route.name || (route.name === 'product' && a.dataset.r === 'products')) a.classList.add('active');
  });
  const fn = routes[route.name] || renderDashboard;
  await fn(route);
}

async function renderLogin() {
  document.body.className = 'admin';
  $('#app').innerHTML = `
    <div class="login-wrap">
      <div class="card login-card">
        <div class="brand" style="margin-bottom:18px">
          <div class="brand-mark">G</div>
          <div><h1>GiftCard Console</h1><p>Admin login</p></div>
        </div>
        <h2>Welcome back</h2>
        <p>Default credentials: admin.username / admin.password</p>
        <div id="err" class="alert err hidden"></div>
        <div class="field"><label>Username</label><input id="u" value="admin" autocomplete="username"/></div>
        <div class="field"><label>Password</label><input id="p" type="password" value="admin123" autocomplete="current-password"/></div>
        <button class="btn" id="go" style="width:100%">Sign in</button>
      </div>
    </div>`;
  const go = async () => {
    $('#err').classList.add('hidden');
    try {
      const r = await api('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username: $('#u').value, password: $('#p').value }),
      });
      setToken(r.token);
      location.hash = '#/dashboard';
    } catch (e) {
      $('#err').textContent = e.message;
      $('#err').classList.remove('hidden');
    }
  };
  $('#go').onclick = go;
  $('#p').onkeydown = (e) => e.key === 'Enter' && go();
}

async function renderDashboard() {
  $('#page-title').textContent = 'Dashboard';
  const s = await api('/dashboard');
  $('#view').innerHTML = `
    <div class="grid stats">
      <div class="card stat"><div class="label">Pending</div><div class="value">${s.pending_orders}</div></div>
      <div class="card stat"><div class="label">Paid orders</div><div class="value">${s.paid_orders}</div></div>
      <div class="card stat"><div class="label">Paid today</div><div class="value">${s.today_paid}</div><div class="hint">${yuan(s.today_amount)}</div></div>
      <div class="card stat"><div class="label">Unused codes</div><div class="value">${s.unused_cards}</div><div class="hint">Products ${s.products} · mock_pay ${s.mock_pay ? 'on' : 'off'}</div></div>
    </div>
    <div class="card" style="margin-top:16px">
      <h3 style="margin:0 0 8px">Quick actions</h3>
      <div class="row">
        <a class="btn" href="#/products">Manage products</a>
        <a class="btn ghost" href="#/orders">View orders</a>
        <a class="btn ghost" href="/shop/" target="_blank">Preview shop</a>
      </div>
      <p class="muted" style="margin:16px 0 0;font-size:13px;line-height:1.6">
        K2: payment method code=<code>giftcard</code>; base_url points here; app_id/api_secret match Merchants.
        Shop orders source=shop auto-deliver; K2 orders source=k2 notify panel.
        Checkout supports Official Alipay and/or 彩虹易支付 — both work for K2 and shop.
      </p>
    </div>`;
}

async function renderProducts() {
  $('#page-title').textContent = 'Products';
  const r = await api('/products');
  const rows = (r.products || []).map(p => `
    <tr>
      <td><b>${esc(p.name)}</b><div class="muted mono">${esc(p.slug)} · ${esc(p.category||'-')} · ${esc(p.region||'')}</div></td>
      <td>${yuan(p.price_cents)}</td>
      <td>${p.enable ? '<span class="badge paid">On sale</span>' : '<span class="badge closed">Off sale</span>'} ${p.badge?`<span class="badge shop">${esc(p.badge)}</span>`:''}</td>
      <td>${p.stock < 0 ? 'Unlimited' : p.stock} <span class="muted">/ Secret ${p.unused_cards ?? 0}</span></td>
      <td class="row">
        <a class="btn sm ghost" href="#/product/${p.id}">Edit</a>
      </td>
    </tr>`).join('') || `<tr><td colspan="5" class="muted">No products</td></tr>`;
  $('#view').innerHTML = `
    <div class="toolbar">
      <div class="muted">Total ${(r.products || []).length}  products · Category apple_id / apple_gc / google / netflix / streaming / data</div>
      <a class="btn" href="#/product/new">Add product</a>
    </div>
    <div class="card table-wrap"><table>
      <thead><tr><th>Name</th><th>Price</th><th>Status</th><th>Stock</th><th></th></tr></thead>
      <tbody>${rows}</tbody>
    </table></div>`;
}

async function renderProductEdit(route) {
  const isNew = !route.id || route.id === 'new';
  $('#page-title').textContent = isNew ? 'Add product' : 'Edit product';
  let p = {
    name: '', slug: '', description: '', category: 'other', region: '', badge: '',
    icon: '', cover_url: '', features: '', price_cents: 1000,
    currency: 'CNY', stock: -1, enable: true, sort: 0, use_card_pool: false,
    deliver_template: 'Thanks for purchasing [{name}]\nOrder: {trade_no}',
  };
  let cardsInfo = '';
  if (!isNew) {
    const r = await api('/products/' + route.id);
    p = r.product;
    cardsInfo = `<div class="muted">Secret：unused ${r.unused_cards} · sold ${r.sold_cards}</div>`;
  }
  $('#view').innerHTML = `
    <div class="card">
      <div class="form-grid">
        <div class="field"><label>Name</label><input id="name" value="${esc(p.name)}"/></div>
        <div class="field"><label>Slug (URL id)</label><input id="slug" value="${esc(p.slug)}" placeholder="auto"/></div>
        <div class="field"><label>Category</label>
          <select id="category">
            ${['apple_id','apple_gc','google','netflix','streaming','data','other'].map(c =>
              `<option value="${c}" ${p.category===c?'selected':''}>${c}</option>`).join('')}
          </select>
        </div>
        <div class="field"><label>Region</label><input id="region" value="${esc(p.region||'')}" placeholder="US"/></div>
        <div class="field"><label>Badge</label><input id="badge" value="${esc(p.badge||'')}" placeholder="Hot"/></div>
        <div class="field"><label>Icon emoji</label><input id="icon" value="${esc(p.icon||'')}" placeholder="🍎"/></div>
        <div class="field full"><label>Description</label><textarea id="desc" rows="3">${esc(p.description)}</textarea></div>
        <div class="field full"><label>Features (one per line)</label><textarea id="features" rows="3">${esc(p.features||'')}</textarea></div>
        <div class="field"><label>Price (cents)</label><input id="price" type="number" value="${p.price_cents}"/></div>
        <div class="field"><label>Sort (higher first)</label><input id="sort" type="number" value="${p.sort || 0}"/></div>
        <div class="field"><label>Stock (-1 unlimited)</label><input id="stock" type="number" value="${p.stock}"/></div>
        <div class="field"><label>Cover URL (optional)</label><input id="cover" value="${esc(p.cover_url || '')}"/></div>
        <div class="field"><label>Delivery mode</label>
          <select id="pool">
            <option value="0" ${!p.use_card_pool ? 'selected' : ''}>Template text</option>
            <option value="1" ${p.use_card_pool ? 'selected' : ''}>Card pool</option>
          </select>
        </div>
        <div class="field"><label>Auto-generate sim secrets</label>
          <select id="autogen">
            <option value="1" ${p.auto_generate !== false ? 'selected' : ''}>Yes (mint by category)</option>
            <option value="0" ${p.auto_generate === false ? 'selected' : ''}>No</option>
          </select>
        </div>
        <div class="field"><label>On sale</label>
          <select id="enable"><option value="1" ${p.enable ? 'selected' : ''}>Yes</option><option value="0" ${!p.enable ? 'selected' : ''}>No</option></select>
        </div>
        <div class="field full"><label>Delivery template ({name} {trade_no})</label>
          <textarea id="tpl" rows="4">${esc(p.deliver_template || '')}</textarea>
        </div>
      </div>
      <div class="row" style="margin-top:8px">
        <button class="btn" id="save">Save</button>
        ${!isNew ? '<button class="btn danger" id="del">Delete</button>' : ''}
        <a class="btn ghost" href="#/products">Back</a>
      </div>
      <div id="msg" class="alert ok hidden" style="margin-top:12px"></div>
      ${!isNew ? `
      <hr style="border:none;border-top:1px solid var(--line);margin:22px 0"/>
      <h3 style="margin:0 0 8px">Import codes</h3>
      ${cardsInfo}
      <div class="field" style="margin-top:10px"><label>One code per line</label><textarea id="codes" rows="5" placeholder="CODE-001&#10;CODE-002"></textarea></div>
      <button class="btn ghost" id="imp">Imported</button>
      <div id="cardlist" class="table-wrap" style="margin-top:14px"></div>
      ` : ''}
    </div>`;

  const payload = () => ({
    name: $('#name').value.trim(),
    slug: $('#slug').value.trim(),
    description: $('#desc').value,
    category: $('#category').value,
    region: $('#region').value.trim(),
    badge: $('#badge').value.trim(),
    icon: $('#icon').value.trim(),
    features: $('#features').value,
    cover_url: $('#cover').value.trim(),
    price_cents: Number($('#price').value),
    stock: Number($('#stock').value),
    sort: Number($('#sort').value),
    enable: $('#enable').value === '1',
    use_card_pool: $('#pool').value === '1',
    auto_generate: $('#autogen').value === '1',
    deliver_template: $('#tpl').value,
    currency: 'CNY',
  });

  $('#save').onclick = async () => {
    try {
      if (isNew) {
        const created = await api('/products', { method: 'POST', body: JSON.stringify(payload()) });
        $('#msg').textContent = 'Created';
        $('#msg').classList.remove('hidden');
        location.hash = '#/product/' + created.id;
      } else {
        await api('/products/' + route.id, { method: 'PUT', body: JSON.stringify(payload()) });
        $('#msg').textContent = 'Saved';
        $('#msg').classList.remove('hidden');
      }
    } catch (e) {
      $('#msg').className = 'alert err';
      $('#msg').textContent = e.message;
      $('#msg').classList.remove('hidden');
    }
  };
  if (!isNew) {
    $('#del').onclick = async () => {
      if (!confirm('Delete this product?')) return;
      await api('/products/' + route.id, { method: 'DELETE' });
      location.hash = '#/products';
    };
    $('#imp').onclick = async () => {
      const r = await api('/products/' + route.id + '/cards', {
        method: 'POST', body: JSON.stringify({ codes: $('#codes').value }),
      });
      alert('Imported ' + r.imported + '  items');
      renderProductEdit(route);
    };
    const cl = await api('/products/' + route.id + '/cards');
    $('#cardlist').innerHTML = `<table><thead><tr><th>Secret</th><th>Status</th></tr></thead><tbody>
      ${(cl.cards || []).map(c => `<tr><td class="mono">${esc(c.code)}</td><td>${badge(c.status)}</td></tr>`).join('') || '<tr><td colspan="2" class="muted">No codes</td></tr>'}
    </tbody></table>`;
  }
}

async function renderOrders() {
  $('#page-title').textContent = 'Orders';
  const status = new URLSearchParams(location.hash.split('?')[1] || '').get('status') || '';
  const r = await api('/orders' + (status ? '?status=' + status : ''));
  const rows = (r.orders || []).map(o => `
    <tr>
      <td class="mono">${esc(o.out_trade_no)}<div class="muted">${esc(o.subject)}</div></td>
      <td>${yuan(o.amount)}</td>
      <td>${badge(o.status)} ${badge(o.source || 'k2')}</td>
      <td class="muted">${o.notify_status || '-'}</td>
      <td class="row">
        ${o.status === 'pending' ? `<button class="btn sm ghost" data-close="${o.id}">Close</button>` : ''}
        ${o.status === 'pending' || o.status === 'expired' ? `<button class="btn sm ghost" data-sync="${o.id}">Sync Alipay</button> <button class="btn sm ghost" data-synce="${o.id}">Sync Epay</button>` : ''}
        ${o.source === 'k2' && (o.status === 'paid' || o.status === 'paid_orphan') ? `<button class="btn sm ghost" data-re="${o.id}">Retry K2 notify</button>` : ''}
        ${o.delivered ? `<button class="btn sm ghost" data-del="${esc(o.delivered)}">Secret</button>` : ''}
      </td>
    </tr>`).join('') || `<tr><td colspan="5" class="muted">No orders</td></tr>`;
  $('#view').innerHTML = `
    <div class="toolbar">
      <div class="row">
        <a class="btn sm ghost" href="#/orders">All</a>
        <a class="btn sm ghost" href="#/orders?status=pending">Pending</a>
        <a class="btn sm ghost" href="#/orders?status=paid">Paid</a>
        <a class="btn sm ghost" href="#/orders?status=paid_orphan">Manual</a>
      </div>
    </div>
    <div class="card table-wrap"><table>
      <thead><tr><th>Orders</th><th>Amount</th><th>Status</th><th>Notify</th><th></th></tr></thead>
      <tbody>${rows}</tbody>
    </table></div>`;
  $$('[data-close]').forEach(b => b.onclick = async () => {
    await api('/orders/' + b.dataset.close + '/close', { method: 'POST' });
    renderOrders();
  });
  $$('[data-re]').forEach(b => b.onclick = async () => {
    await api('/orders/' + b.dataset.re + '/renotify', { method: 'POST' });
    alert('Notify re-queued');
  });
  $$('[data-sync]').forEach(b => b.onclick = async () => {
    try {
      const o = await api('/orders/' + b.dataset.sync + '/sync-alipay', { method: 'POST' });
      alert('Sync result: ' + (o.status || JSON.stringify(o)));
      renderOrders();
    } catch (e) { alert(e.message); }
  });
  $$('[data-synce]').forEach(b => b.onclick = async () => {
    try {
      const o = await api('/orders/' + b.dataset.synce + '/sync-epay', { method: 'POST' });
      alert('Epay sync: ' + (o.status || JSON.stringify(o)));
      renderOrders();
    } catch (e) { alert(e.message); }
  });
  $$('[data-del]').forEach(b => b.onclick = () => alert(b.dataset.del));
}

async function renderMerchants() {
  $('#page-title').textContent = 'Merchants / K2 integration';
  const r = await api('/merchants');
  const rows = (r.merchants || []).map(m => `
    <tr>
      <td><b>${esc(m.name || m.app_id)}</b><div class="mono muted">${esc(m.app_id)}</div></td>
      <td>${m.enable ? badge('paid') : badge('closed')}</td>
      <td><button class="btn sm ghost" data-id="${m.id}" data-en="${m.enable ? 0 : 1}">${m.enable ? 'Disable' : 'Enable'}</button></td>
    </tr>`).join('');
  $('#view').innerHTML = `
    <div class="card" style="margin-bottom:16px">
      <h3 style="margin-top:0">Add merchant</h3>
      <div class="form-grid">
        <div class="field"><label>App ID</label><input id="app_id" placeholder="k2-main"/></div>
        <div class="field"><label>Name</label><input id="mname" placeholder="K2Board"/></div>
        <div class="field full"><label>API Secret</label><input id="secret" placeholder="and  K2 giftcard config match"/></div>
      </div>
      <button class="btn" id="addm">Create</button>
    </div>
    <div class="card table-wrap"><table>
      <thead><tr><th>Merchant</th><th>Status</th><th></th></tr></thead>
      <tbody>${rows || '<tr><td colspan="3" class="muted">None</td></tr>'}</tbody>
    </table></div>`;
  $('#addm').onclick = async () => {
    await api('/merchants', {
      method: 'POST',
      body: JSON.stringify({ app_id: $('#app_id').value, name: $('#mname').value, api_secret: $('#secret').value }),
    });
    renderMerchants();
  };
  $$('[data-id]').forEach(b => b.onclick = async () => {
    await api('/merchants/' + b.dataset.id, {
      method: 'PUT', body: JSON.stringify({ enable: b.dataset.en === '1' }),
    });
    renderMerchants();
  });
}

async function renderSettings() {
  $('#page-title').textContent = 'Settings';
  const s = await api('/settings');
  $('#view').innerHTML = `
    <div class="card">
      <div class="form-grid">
        <div class="field"><label>Site name</label><input value="${esc(s.site_name)}" disabled/></div>
        <div class="field"><label>Public Base URL</label><input value="${esc(s.public_base_url)}" disabled/></div>
        <div class="field"><label>Shop title</label><input value="${esc(s.shop_title)}" disabled/></div>
        <div class="field"><label>Payment status</label><input value="${s.alipay_configured ? 'Alipay ready' : (s.mock_pay ? 'Mock pay' : 'Not configured')}" disabled/></div>
        <div class="field full"><label>Alipay notify</label><input value="${esc(s.alipay_notify_url||'')}" disabled/></div>
      </div>
      <p class="muted" style="font-size:13px;line-height:1.6">
        Set Alipay keys under <a href="#/payment" style="color:#93b4ff">Payment</a>  — saved live, no yaml edit required.
        <code>public_base_url</code> Set via install script / config (public HTTPS).
      </p>
      <hr style="border:none;border-top:1px solid var(--line);margin:18px 0"/>
      <h3 style="margin:0 0 12px">Change password</h3>
      <div class="form-grid">
        <div class="field"><label>Current password</label><input type="password" id="op"/></div>
        <div class="field"><label>New password</label><input type="password" id="np"/></div>
      </div>
      <button class="btn" id="cp">Update password</button>
      <div id="msg" class="alert ok hidden" style="margin-top:12px"></div>
    </div>`;
  $('#cp').onclick = async () => {
    try {
      await api('/auth/password', {
        method: 'POST',
        body: JSON.stringify({ old_password: $('#op').value, new_password: $('#np').value }),
      });
      $('#msg').textContent = 'Password updated';
      $('#msg').className = 'alert ok';
      $('#msg').classList.remove('hidden');
    } catch (e) {
      $('#msg').textContent = e.message;
      $('#msg').className = 'alert err';
      $('#msg').classList.remove('hidden');
    }
  };
}

async function renderPayment() {
  $('#page-title').textContent = 'Payment';
  const [p, e] = await Promise.all([api('/settings/payment'), api('/settings/epay')]);
  $('#view').innerHTML = `
    <div class="card">
      <h3 style="margin-top:0">Official Alipay</h3>
      <p class="muted" style="margin-top:0;font-size:13px;line-height:1.6">
        Secrets stay on server; private keys are not re-displayed.
        Notify URL：<code>${esc(p.notify_url)}</code>
      </p>
      <div class="form-grid">
        <div class="field"><label>Enable Alipay</label>
          <select id="en"><option value="1" ${p.enabled!==false?'selected':''}>Yes</option><option value="0" ${p.enabled===false?'selected':''}>No</option></select>
        </div>
        <div class="field"><label>Mock pay（local testing）</label>
          <select id="mock"><option value="1" ${p.mock_pay?'selected':''}>On</option><option value="0" ${!p.mock_pay?'selected':''}>Off</option></select>
        </div>
        <div class="field"><label>App ID</label><input id="appid" value="${esc(p.app_id||'')}" placeholder="20xxxxxxxxxxxx"/></div>
        <div class="field"><label>Product</label>
          <select id="prod">
            <option value="page" ${p.product==='page'?'selected':''}>Desktop page</option>
            <option value="wap" ${p.product==='wap'?'selected':''}>Mobile wap</option>
            <option value="auto" ${p.product==='auto'?'selected':''}>Auto</option>
          </select>
        </div>
        <div class="field"><label>Environment</label>
          <select id="prodmode"><option value="0" ${!p.is_production?'selected':''}>Sandbox</option><option value="1" ${p.is_production?'selected':''}>Production</option></select>
        </div>
        <div class="field"><label>Timeout</label><input id="timeout" value="${esc(p.timeout_express||'30m')}"/></div>
        <div class="field full"><label>Bill subject (neutral)</label><input id="bill" value="${esc(p.bill_subject||'Products')}"/></div>
        <div class="field full"><label>App private key PEM ${p.has_private_key?'（set — leave blank to keep）':''}</label>
          <textarea id="priv" rows="5" placeholder="-----BEGIN RSA PRIVATE KEY-----"></textarea>
        </div>
        <div class="field full"><label>Alipay public key PEM ${p.has_public_key?'（set — leave blank to keep）':''}</label>
          <textarea id="pub" rows="5" placeholder="-----BEGIN PUBLIC KEY-----"></textarea>
        </div>
      </div>
      <div class="row">
        <button class="btn" id="savePay">Save Alipay</button>
      </div>
      <div id="payMsg" class="alert ok hidden" style="margin-top:12px"></div>
    </div>
    <div class="card" style="margin-top:16px">
      <h3 style="margin-top:0">彩虹易支付（V1 易支付）</h3>
      <p class="muted" style="margin-top:0;font-size:13px;line-height:1.6">
        Works for K2 giftcard and shop checkout. After pay, platform notifies K2 as before.
        Notify URL（填到易支付后台）：<code>${esc(e.notify_url)}</code><br/>
        Return URL：<code>${esc(e.return_url)}</code>
      </p>
      <div class="form-grid">
        <div class="field"><label>Enable Epay</label>
          <select id="een"><option value="1" ${e.enabled?'selected':''}>Yes</option><option value="0" ${!e.enabled?'selected':''}>No</option></select>
        </div>
        <div class="field"><label>商户 ID (pid)</label><input id="epid" value="${esc(e.pid||'')}" placeholder="1000"/></div>
        <div class="field full"><label>API 地址（不含 submit.php）</label><input id="eurl" value="${esc(e.api_url||'')}" placeholder="https://pay.example.com"/></div>
        <div class="field full"><label>商户密钥 (key) ${e.has_key?'（已设置 — 留空保持不变）':''}</label>
          <input id="ekey" type="password" autocomplete="new-password" placeholder="${e.has_key?'••••••••':''}"/>
        </div>
        <div class="field"><label>通道 types</label><input id="etypes" value="${esc(e.types||'alipay,wxpay')}" placeholder="alipay,wxpay"/></div>
        <div class="field"><label>账单商品名</label><input id="ename" value="${esc(e.name||'Digital Goods')}"/></div>
      </div>
      <div class="row">
        <button class="btn" id="saveEpay">Save Epay</button>
        <a class="btn ghost" href="/shop/" target="_blank">Open shop</a>
      </div>
      <div id="epayMsg" class="alert ok hidden" style="margin-top:12px"></div>
      <p class="muted" style="font-size:12px;margin-top:16px">Public Base URL：${esc(e.public_base_url || p.public_base_url)}（须公网可访问，供易支付异步回调）</p>
    </div>`;
  $('#savePay').onclick = async () => {
    try {
      const body = {
        enabled: $('#en').value === '1',
        mock_pay: $('#mock').value === '1',
        app_id: $('#appid').value.trim(),
        product: $('#prod').value,
        is_production: $('#prodmode').value === '1',
        timeout_express: $('#timeout').value.trim() || '30m',
        bill_subject: $('#bill').value.trim() || 'Products',
        private_key: $('#priv').value.trim(),
        alipay_public_key: $('#pub').value.trim(),
      };
      const r = await api('/settings/payment', { method: 'PUT', body: JSON.stringify(body) });
      $('#payMsg').textContent = 'Saved. Alipay active=' + !!(r.payment && r.payment.effective_enabled) + ' mock=' + !!(r.payment && r.payment.mock_pay);
      $('#payMsg').className = 'alert ok';
      $('#payMsg').classList.remove('hidden');
      $('#priv').value = '';
      $('#pub').value = '';
    } catch (err) {
      $('#payMsg').textContent = err.message;
      $('#payMsg').className = 'alert err';
      $('#payMsg').classList.remove('hidden');
    }
  };
  $('#saveEpay').onclick = async () => {
    try {
      const body = {
        enabled: $('#een').value === '1',
        api_url: $('#eurl').value.trim(),
        pid: $('#epid').value.trim(),
        key: $('#ekey').value.trim(),
        types: $('#etypes').value.trim() || 'alipay',
        name: $('#ename').value.trim() || 'Digital Goods',
      };
      const r = await api('/settings/epay', { method: 'PUT', body: JSON.stringify(body) });
      $('#epayMsg').textContent = 'Saved. Epay active=' + !!(r.epay && r.epay.effective_enabled) + ' types=' + (r.epay && r.epay.types || '');
      $('#epayMsg').className = 'alert ok';
      $('#epayMsg').classList.remove('hidden');
      $('#ekey').value = '';
    } catch (err) {
      $('#epayMsg').textContent = err.message;
      $('#epayMsg').className = 'alert err';
      $('#epayMsg').classList.remove('hidden');
    }
  };
}

boot();
