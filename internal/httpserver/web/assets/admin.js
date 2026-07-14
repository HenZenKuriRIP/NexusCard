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
function statusLabel(status) {
  const m = {
    pending: '待支付', paid: '已支付', closed: '已关闭', expired: '已过期',
    paid_orphan: '异常已付', unused: '未售', sold: '已售', shop: '商城', k2: 'K2',
  };
  return m[status] || status || '-';
}
function badge(status) {
  return `<span class="badge ${status || ''}">${esc(statusLabel(status))}</span>`;
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
    const raw = r.site_name || '卡卡基地';
    window.__site = /nexuscard/i.test(raw) ? '卡卡基地' : raw;
  } catch {
    setToken('');
    location.hash = '#/login';
    return;
  }
  $('#app').innerHTML = `
    <div class="admin-layout">
      <aside class="sidebar">
        <div class="brand">
          <div class="brand-mark">卡</div>
          <div>
            <h1>${esc(window.__site || '卡卡基地')}</h1>
            <p>管理后台</p>
          </div>
        </div>
        <nav class="nav" id="nav">
          <a href="#/dashboard" data-r="dashboard">仪表盘</a>
          <a href="#/products" data-r="products">商品</a>
          <a href="#/orders" data-r="orders">订单</a>
          <a href="#/merchants" data-r="merchants">商户 / K2</a>
          <a href="#/settings" data-r="settings">设置</a>
          <a href="#/payment" data-r="payment">支付配置</a>
          <a href="/shop/" target="_blank">打开商城 ↗</a>
        </nav>
      </aside>
      <section class="main">
        <div class="topbar">
          <h2 id="page-title">—</h2>
          <div class="user">${esc(me.display_name || me.username)} · <a href="#" id="logout">退出登录</a></div>
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
          <div class="brand-mark">N</div>
          <div><h1>卡卡基地控制台</h1><p>管理员登录</p></div>
        </div>
        <h2>欢迎回来</h2>
        <p class="muted" style="margin:0 0 16px;font-size:13px;line-height:1.5">
          用户名默认 <code>admin</code>；密码见服务器
          <code>/opt/giftcard-platform/README-DEPLOY.txt</code>（安装时随机生成，不是 admin123）
        </p>
        <div id="err" class="alert err hidden"></div>
        <form id="loginForm" autocomplete="on">
          <div class="field"><label>用户名</label>
            <input id="u" name="username" type="text" value="admin" autocomplete="username" autocapitalize="off" spellcheck="false"/>
          </div>
          <div class="field"><label>密码</label>
            <input id="p" name="password" type="password" value="" placeholder="粘贴部署时生成的密码" autocomplete="current-password"/>
          </div>
          <button class="btn" id="go" type="submit" style="width:100%">登录</button>
        </form>
      </div>
    </div>`;
  const form = $('#loginForm');
  const go = async (ev) => {
    if (ev) ev.preventDefault();
    $('#err').classList.add('hidden');
    const username = ($('#u').value || '').trim();
    const password = $('#p').value || '';
    if (!username || !password) {
      $('#err').textContent = '请输入用户名和密码';
      $('#err').classList.remove('hidden');
      return;
    }
    try {
      const r = await api('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      });
      setToken(r.token);
      location.hash = '#/dashboard';
    } catch (e) {
      $('#err').textContent = e.message || '登录失败';
      $('#err').classList.remove('hidden');
      $('#p').focus();
      $('#p').select();
    }
  };
  form.onsubmit = go;
}

async function renderDashboard() {
  $('#page-title').textContent = '仪表盘';
  const s = await api('/dashboard');
  $('#view').innerHTML = `
    <div class="grid stats">
      <div class="card stat"><div class="label">待支付</div><div class="value">${s.pending_orders}</div></div>
      <div class="card stat"><div class="label">已支付订单</div><div class="value">${s.paid_orders}</div></div>
      <div class="card stat"><div class="label">今日成交</div><div class="value">${s.today_paid}</div><div class="hint">${yuan(s.today_amount)}</div></div>
      <div class="card stat"><div class="label">未售卡密</div><div class="value">${s.unused_cards}</div><div class="hint">商品 ${s.products} · 模拟支付 ${s.mock_pay ? '开' : '关'}</div></div>
    </div>
    <div class="card" style="margin-top:16px">
      <h3 style="margin:0 0 8px">快捷操作</h3>
      <div class="row">
        <a class="btn" href="#/products">管理商品</a>
        <a class="btn ghost" href="#/orders">查看订单</a>
        <a class="btn ghost" href="/shop/" target="_blank">预览商城</a>
      </div>
      <p class="muted" style="margin:16px 0 0;font-size:13px;line-height:1.6">
        K2 对接：支付方式 code=<code>giftcard</code>，base_url 指向本站，app_id / api_secret 与「商户 / K2」一致。
        商城订单 source=shop 自动发货；K2 订单 source=k2 异步通知面板。
        收银台支持官方支付宝和/或彩虹易支付。
      </p>
    </div>`;
}

async function renderProducts() {
  $('#page-title').textContent = '商品';
  const r = await api('/products');
  const rows = (r.products || []).map(p => `
    <tr>
      <td><b>${esc(p.name)}</b><div class="muted mono">${esc(p.slug)} · ${esc(p.category||'-')} · ${esc(p.region||'')}</div></td>
      <td>${yuan(p.price_cents)}</td>
      <td>${p.enable ? '<span class="badge paid">在售</span>' : '<span class="badge closed">下架</span>'} ${p.badge?`<span class="badge shop">${esc(p.badge)}</span>`:''}</td>
      <td>${p.stock < 0 ? '不限' : p.stock} <span class="muted">/ 卡密 ${p.unused_cards ?? 0}</span></td>
      <td class="row">
        <a class="btn sm ghost" href="#/product/${p.id}">编辑</a>
      </td>
    </tr>`).join('') || `<tr><td colspan="5" class="muted">暂无商品</td></tr>`;
  $('#view').innerHTML = `
    <div class="toolbar">
      <div class="muted">共 ${(r.products || []).length} 个商品 · 分类：美区 ID / 礼品卡 / Google / Netflix / 流媒体 / 软件</div>
      <a class="btn" href="#/product/new">添加商品</a>
    </div>
    <div class="card table-wrap"><table>
      <thead><tr><th>名称</th><th>价格</th><th>状态</th><th>库存</th><th></th></tr></thead>
      <tbody>${rows}</tbody>
    </table></div>`;
}

async function renderProductEdit(route) {
  const isNew = !route.id || route.id === 'new';
  $('#page-title').textContent = isNew ? '添加商品' : '编辑商品';
  let p = {
    name: '', slug: '', description: '', category: 'other', region: '', badge: '',
    icon: '', cover_url: '', features: '', price_cents: 1000,
    currency: 'CNY', stock: -1, enable: true, sort: 0, use_card_pool: false,
    deliver_template: '感谢购买【{name}】\n订单号：{trade_no}',
  };
  let cardsInfo = '';
  if (!isNew) {
    const r = await api('/products/' + route.id);
    p = r.product;
    cardsInfo = `<div class="muted">卡密：未售 ${r.unused_cards} · 已售 ${r.sold_cards}</div>`;
  }
  const catLabels = {
    apple_id: '美区 Apple ID', apple_gc: 'App Store 礼品卡', google: 'Google 账号',
    netflix: 'Netflix', streaming: '流媒体', other: '软件 / 其他',
  };
  $('#view').innerHTML = `
    <div class="card">
      <div class="form-grid">
        <div class="field"><label>名称</label><input id="name" value="${esc(p.name)}"/></div>
        <div class="field"><label>Slug（URL 标识）</label><input id="slug" value="${esc(p.slug)}" placeholder="留空自动生成"/></div>
        <div class="field"><label>分类</label>
          <select id="category">
            ${['apple_id','apple_gc','google','netflix','streaming','other'].map(c =>
              `<option value="${c}" ${p.category===c?'selected':''}>${catLabels[c]||c}</option>`).join('')}
          </select>
        </div>
        <div class="field"><label>地区</label><input id="region" value="${esc(p.region||'')}" placeholder="US"/></div>
        <div class="field"><label>角标</label><input id="badge" value="${esc(p.badge||'')}" placeholder="热销"/></div>
        <div class="field"><label>图标 emoji</label><input id="icon" value="${esc(p.icon||'')}" placeholder="🍎"/></div>
        <div class="field full"><label>描述</label><textarea id="desc" rows="3">${esc(p.description)}</textarea></div>
        <div class="field full"><label>卖点（每行一条）</label><textarea id="features" rows="3">${esc(p.features||'')}</textarea></div>
        <div class="field"><label>价格（分）</label><input id="price" type="number" value="${p.price_cents}"/></div>
        <div class="field"><label>排序（越大越靠前）</label><input id="sort" type="number" value="${p.sort || 0}"/></div>
        <div class="field"><label>库存（-1 不限）</label><input id="stock" type="number" value="${p.stock}"/></div>
        <div class="field"><label>封面 URL（可选）</label><input id="cover" value="${esc(p.cover_url || '')}"/></div>
        <div class="field"><label>发货模式</label>
          <select id="pool">
            <option value="0" ${!p.use_card_pool ? 'selected' : ''}>模板文本</option>
            <option value="1" ${p.use_card_pool ? 'selected' : ''}>卡密池</option>
          </select>
        </div>
        <div class="field"><label>自动生成模拟卡密</label>
          <select id="autogen">
            <option value="1" ${p.auto_generate !== false ? 'selected' : ''}>是（按分类生成）</option>
            <option value="0" ${p.auto_generate === false ? 'selected' : ''}>否</option>
          </select>
        </div>
        <div class="field"><label>上架</label>
          <select id="enable"><option value="1" ${p.enable ? 'selected' : ''}>是</option><option value="0" ${!p.enable ? 'selected' : ''}>否</option></select>
        </div>
        <div class="field full"><label>发货模板（可用 {name} {trade_no}）</label>
          <textarea id="tpl" rows="4">${esc(p.deliver_template || '')}</textarea>
        </div>
      </div>
      <div class="row" style="margin-top:8px">
        <button class="btn" id="save">保存</button>
        ${!isNew ? '<button class="btn danger" id="del">删除</button>' : ''}
        <a class="btn ghost" href="#/products">返回</a>
      </div>
      <div id="msg" class="alert ok hidden" style="margin-top:12px"></div>
      ${!isNew ? `
      <hr style="border:none;border-top:1px solid var(--line);margin:22px 0"/>
      <h3 style="margin:0 0 8px">导入卡密</h3>
      ${cardsInfo}
      <div class="field" style="margin-top:10px"><label>每行一个卡密</label><textarea id="codes" rows="5" placeholder="CODE-001&#10;CODE-002"></textarea></div>
      <button class="btn ghost" id="imp">导入</button>
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
        $('#msg').textContent = '已创建';
        $('#msg').classList.remove('hidden');
        location.hash = '#/product/' + created.id;
      } else {
        await api('/products/' + route.id, { method: 'PUT', body: JSON.stringify(payload()) });
        $('#msg').textContent = '已保存';
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
      if (!confirm('确定删除该商品？')) return;
      await api('/products/' + route.id, { method: 'DELETE' });
      location.hash = '#/products';
    };
    $('#imp').onclick = async () => {
      const r = await api('/products/' + route.id + '/cards', {
        method: 'POST', body: JSON.stringify({ codes: $('#codes').value }),
      });
      alert('已导入 ' + r.imported + ' 条');
      renderProductEdit(route);
    };
    const cl = await api('/products/' + route.id + '/cards');
    $('#cardlist').innerHTML = `<table><thead><tr><th>卡密</th><th>状态</th></tr></thead><tbody>
      ${(cl.cards || []).map(c => `<tr><td class="mono">${esc(c.code)}</td><td>${badge(c.status)}</td></tr>`).join('') || '<tr><td colspan="2" class="muted">暂无卡密</td></tr>'}
    </tbody></table>`;
  }
}

async function renderOrders() {
  $('#page-title').textContent = '订单';
  const status = new URLSearchParams(location.hash.split('?')[1] || '').get('status') || '';
  const r = await api('/orders' + (status ? '?status=' + status : ''));
  const rows = (r.orders || []).map(o => `
    <tr>
      <td class="mono">${esc(o.out_trade_no)}<div class="muted">${esc(o.subject)}</div></td>
      <td>${yuan(o.amount)}</td>
      <td>${badge(o.status)} ${badge(o.source || 'k2')}</td>
      <td class="muted">${esc(statusLabel(o.notify_status) === o.notify_status ? (o.notify_status || '-') : statusLabel(o.notify_status))}</td>
      <td class="row">
        ${o.status === 'pending' ? `<button class="btn sm ghost" data-close="${o.id}">关闭</button>` : ''}
        ${o.status === 'pending' || o.status === 'expired' ? `<button class="btn sm ghost" data-sync="${o.id}">同步支付宝</button> <button class="btn sm ghost" data-synce="${o.id}">同步易支付</button>` : ''}
        ${o.source === 'k2' && (o.status === 'paid' || o.status === 'paid_orphan') ? `<button class="btn sm ghost" data-re="${o.id}">重试 K2 通知</button>` : ''}
        ${o.delivered ? `<button class="btn sm ghost" data-del="${esc(o.delivered)}">查看卡密</button>` : ''}
      </td>
    </tr>`).join('') || `<tr><td colspan="5" class="muted">暂无订单</td></tr>`;
  $('#view').innerHTML = `
    <div class="toolbar">
      <div class="row">
        <a class="btn sm ghost" href="#/orders">全部</a>
        <a class="btn sm ghost" href="#/orders?status=pending">待支付</a>
        <a class="btn sm ghost" href="#/orders?status=paid">已支付</a>
        <a class="btn sm ghost" href="#/orders?status=paid_orphan">异常已付</a>
      </div>
    </div>
    <div class="card table-wrap"><table>
      <thead><tr><th>订单</th><th>金额</th><th>状态</th><th>通知</th><th></th></tr></thead>
      <tbody>${rows}</tbody>
    </table></div>`;
  $$('[data-close]').forEach(b => b.onclick = async () => {
    await api('/orders/' + b.dataset.close + '/close', { method: 'POST' });
    renderOrders();
  });
  $$('[data-re]').forEach(b => b.onclick = async () => {
    await api('/orders/' + b.dataset.re + '/renotify', { method: 'POST' });
    alert('已重新排队通知 K2');
  });
  $$('[data-sync]').forEach(b => b.onclick = async () => {
    try {
      const o = await api('/orders/' + b.dataset.sync + '/sync-alipay', { method: 'POST' });
      alert('同步结果：' + statusLabel(o.status || '') + (o.status ? '' : JSON.stringify(o)));
      renderOrders();
    } catch (e) { alert(e.message); }
  });
  $$('[data-synce]').forEach(b => b.onclick = async () => {
    try {
      const o = await api('/orders/' + b.dataset.synce + '/sync-epay', { method: 'POST' });
      alert('易支付同步：' + statusLabel(o.status || '') + (o.status ? '' : JSON.stringify(o)));
      renderOrders();
    } catch (e) { alert(e.message); }
  });
  $$('[data-del]').forEach(b => b.onclick = () => alert(b.dataset.del));
}

async function renderMerchants() {
  $('#page-title').textContent = '商户 / K2 对接';
  const r = await api('/merchants');
  const rows = (r.merchants || []).map(m => `
    <tr>
      <td><b>${esc(m.name || m.app_id)}</b><div class="mono muted">${esc(m.app_id)}</div></td>
      <td>${m.enable ? '<span class="badge paid">启用</span>' : '<span class="badge closed">停用</span>'}</td>
      <td><button class="btn sm ghost" data-id="${m.id}" data-en="${m.enable ? 0 : 1}">${m.enable ? '停用' : '启用'}</button></td>
    </tr>`).join('');
  $('#view').innerHTML = `
    <div class="card" style="margin-bottom:16px">
      <h3 style="margin-top:0">添加商户</h3>
      <div class="form-grid">
        <div class="field"><label>App ID</label><input id="app_id" placeholder="k2-main"/></div>
        <div class="field"><label>名称</label><input id="mname" placeholder="K2Board"/></div>
        <div class="field full"><label>API Secret</label><input id="secret" placeholder="需与 K2 giftcard 配置一致"/></div>
      </div>
      <button class="btn" id="addm">创建</button>
    </div>
    <div class="card table-wrap"><table>
      <thead><tr><th>商户</th><th>状态</th><th></th></tr></thead>
      <tbody>${rows || '<tr><td colspan="3" class="muted">暂无</td></tr>'}</tbody>
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
  $('#page-title').textContent = '设置';
  const s = await api('/settings');
  const payStatus = s.alipay_configured ? '支付宝已就绪' : (s.epay_configured ? '易支付已就绪' : (s.mock_pay ? '模拟支付' : '未配置'));
  $('#view').innerHTML = `
    <div class="card">
      <div class="form-grid">
        <div class="field"><label>站点名称</label><input value="${esc(s.site_name)}" disabled/></div>
        <div class="field"><label>公网地址</label><input value="${esc(s.public_base_url)}" disabled/></div>
        <div class="field"><label>商城标题</label><input value="${esc(s.shop_title)}" disabled/></div>
        <div class="field"><label>支付状态</label><input value="${esc(payStatus)}" disabled/></div>
        <div class="field full"><label>支付宝回调</label><input value="${esc(s.alipay_notify_url||'')}" disabled/></div>
      </div>
      <p class="muted" style="font-size:13px;line-height:1.6">
        请在 <a href="#/payment" style="color:#93b4ff">支付配置</a> 中填写支付宝 / 易支付密钥，保存后即时生效。
        <code>public_base_url</code> 由安装脚本或配置文件设置（须为公网 HTTPS）。
      </p>
      <hr style="border:none;border-top:1px solid var(--line);margin:18px 0"/>
      <h3 style="margin:0 0 12px">修改密码</h3>
      <div class="form-grid">
        <div class="field"><label>当前密码</label><input type="password" id="op"/></div>
        <div class="field"><label>新密码</label><input type="password" id="np"/></div>
      </div>
      <button class="btn" id="cp">更新密码</button>
      <div id="msg" class="alert ok hidden" style="margin-top:12px"></div>
    </div>`;
  $('#cp').onclick = async () => {
    try {
      await api('/auth/password', {
        method: 'POST',
        body: JSON.stringify({ old_password: $('#op').value, new_password: $('#np').value }),
      });
      $('#msg').textContent = '密码已更新';
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
  $('#page-title').textContent = '支付配置';
  let p = {};
  let e = {};
  try {
    p = await api('/settings/payment') || {};
  } catch (err) {
    $('#view').innerHTML = `<div class="card"><div class="alert err">加载支付宝配置失败: ${esc(err.message)}</div></div>`;
    return;
  }
  try {
    e = await api('/settings/epay') || {};
  } catch (err) {
    e = { _error: err.message, enabled: false, types: 'alipay,wxpay', name: '数字商品' };
  }
  $('#view').innerHTML = `
    ${e._error ? `<div class="alert err" style="margin-bottom:12px">易支付配置接口暂不可用: ${esc(e._error)}（请升级到含 epay 的版本）</div>` : ''}
    <div class="card">
      <h3 style="margin-top:0">官方支付宝</h3>
      <p class="muted" style="margin-top:0;font-size:13px;line-height:1.6">
        密钥仅保存在服务器，不会再次回显。
        异步通知 URL：<code>${esc(p.notify_url)}</code>
      </p>
      <div class="form-grid">
        <div class="field"><label>启用支付宝</label>
          <select id="en"><option value="1" ${p.enabled!==false?'selected':''}>是</option><option value="0" ${p.enabled===false?'selected':''}>否</option></select>
        </div>
        <div class="field"><label>模拟支付（本地测试）</label>
          <select id="mock"><option value="1" ${p.mock_pay?'selected':''}>开</option><option value="0" ${!p.mock_pay?'selected':''}>关</option></select>
        </div>
        <div class="field"><label>App ID</label><input id="appid" value="${esc(p.app_id||'')}" placeholder="20xxxxxxxxxxxx"/></div>
        <div class="field"><label>产品类型</label>
          <select id="prod">
            <option value="page" ${p.product==='page'?'selected':''}>电脑网页</option>
            <option value="wap" ${p.product==='wap'?'selected':''}>手机 WAP</option>
            <option value="auto" ${p.product==='auto'?'selected':''}>自动识别</option>
          </select>
        </div>
        <div class="field"><label>环境</label>
          <select id="prodmode"><option value="0" ${!p.is_production?'selected':''}>沙箱</option><option value="1" ${p.is_production?'selected':''}>正式</option></select>
        </div>
        <div class="field"><label>超时时间</label><input id="timeout" value="${esc(p.timeout_express||'30m')}"/></div>
        <div class="field full"><label>账单标题（中性文案）</label><input id="bill" value="${esc(p.bill_subject||'数字商品')}"/></div>
        <div class="field full"><label>应用私钥 PEM ${p.has_private_key?'（已设置 — 留空保持不变）':''}</label>
          <textarea id="priv" rows="5" placeholder="-----BEGIN RSA PRIVATE KEY-----"></textarea>
        </div>
        <div class="field full"><label>支付宝公钥 PEM ${p.has_public_key?'（已设置 — 留空保持不变）':''}</label>
          <textarea id="pub" rows="5" placeholder="-----BEGIN PUBLIC KEY-----"></textarea>
        </div>
      </div>
      <div class="row">
        <button class="btn" id="savePay">保存支付宝</button>
      </div>
      <div id="payMsg" class="alert ok hidden" style="margin-top:12px"></div>
    </div>
    <div class="card" style="margin-top:16px">
      <h3 style="margin-top:0">彩虹易支付（V1 易支付）</h3>
      <p class="muted" style="margin-top:0;font-size:13px;line-height:1.6">
        同时适用于 K2 giftcard 与商城收银台；支付成功后仍按原逻辑通知 K2。
        异步通知（填到易支付后台）：<code>${esc(e.notify_url)}</code><br/>
        同步跳转：<code>${esc(e.return_url)}</code>
      </p>
      <div class="form-grid">
        <div class="field"><label>启用易支付</label>
          <select id="een"><option value="1" ${e.enabled?'selected':''}>是</option><option value="0" ${!e.enabled?'selected':''}>否</option></select>
        </div>
        <div class="field"><label>商户 ID (pid)</label><input id="epid" value="${esc(e.pid||'')}" placeholder="1000"/></div>
        <div class="field full"><label>API 地址（不含 submit.php）</label><input id="eurl" value="${esc(e.api_url||'')}" placeholder="https://pay.example.com"/></div>
        <div class="field full"><label>商户密钥 (key) ${e.has_key?'（已设置 — 留空保持不变）':''}</label>
          <input id="ekey" type="password" autocomplete="new-password" placeholder="${e.has_key?'••••••••':''}"/>
        </div>
        <div class="field"><label>通道 types</label><input id="etypes" value="${esc(e.types||'alipay,wxpay')}" placeholder="alipay,wxpay"/></div>
        <div class="field"><label>账单商品名</label><input id="ename" value="${esc(e.name||'数字商品')}"/></div>
      </div>
      <div class="row">
        <button class="btn" id="saveEpay">保存易支付</button>
        <a class="btn ghost" href="/shop/" target="_blank">打开商城</a>
      </div>
      <div id="epayMsg" class="alert ok hidden" style="margin-top:12px"></div>
      <p class="muted" style="font-size:12px;margin-top:16px">公网地址：${esc(e.public_base_url || p.public_base_url)}（须公网可访问，供易支付异步回调）</p>
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
        bill_subject: $('#bill').value.trim() || '数字商品',
        private_key: $('#priv').value.trim(),
        alipay_public_key: $('#pub').value.trim(),
      };
      const r = await api('/settings/payment', { method: 'PUT', body: JSON.stringify(body) });
      $('#payMsg').textContent = '已保存。支付宝生效=' + !!(r.payment && r.payment.effective_enabled) + ' 模拟支付=' + !!(r.payment && r.payment.mock_pay);
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
        name: $('#ename').value.trim() || '数字商品',
      };
      const r = await api('/settings/epay', { method: 'PUT', body: JSON.stringify(body) });
      $('#epayMsg').textContent = '已保存。易支付生效=' + !!(r.epay && r.epay.effective_enabled) + ' 通道=' + (r.epay && r.epay.types || '');
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
