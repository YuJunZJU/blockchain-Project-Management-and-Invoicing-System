const state = { invoices: [], principal: null, projects: [], reimbursements: [], users: [], editingProjectId: null, ocrFile: null, ocrSuggestion: null };
const $ = (selector) => document.querySelector(selector);
const api = async (path, options = {}) => {
  const defaultHeaders = options.body instanceof FormData ? {} : { 'Content-Type': 'application/json' };
  const response = await fetch(`/api${path}`, { credentials: 'same-origin', headers: { ...defaultHeaders, ...(options.headers || {}) }, ...options });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    if (response.status === 401 && path !== '/auth/me') showLogin();
    throw new Error(data.error || '请求失败，请检查 Fabric 网络和服务日志');
  }
  return data;
};
const cents = (value) => Math.round(Number(value || 0) * 100);
const money = (value, currency = 'CNY') => new Intl.NumberFormat('zh-CN', { style: 'currency', currency, minimumFractionDigits: 2 }).format(value / 100);
const safe = (value = '') => String(value).replace(/[&<>'"]/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[char]));
const statusText = (status) => ({ ISSUED: '已开具', IN_CIRCULATION: '流转中', VOIDED: '已作废' }[status] || status);
const projectStatusText = (status) => ({ DRAFT: '草稿', PENDING_REVIEW: '待立项审核', REVISION_REQUIRED: '需修改', EXECUTING: '执行中', CLOSURE_REVIEW: '待结项审核', CLOSURE_APPROVED: '结项验收通过' }[status] || status);
const reimbursementStatusText = (status) => ({ PENDING_REVIEW: '待报销审核', REVISION_REQUIRED: '材料需修改', APPROVED_RESERVED: '已审核，额度已冻结', PAID: '已支付' }[status] || status);
const roleText = (role) => ({ ISSUER: '开票员', HOLDER: '跨组织流转员', AUDITOR: '审计员', PROJECT_MEMBER: '项目组成员', PROJECT_REVIEWER: '项目管理审核员', FINANCE_ADMIN: '财务管理员' }[role] || role);
const organizationText = (mspId) => ({ Org1MSP: 'Org1 · 开票组织', Org2MSP: 'Org2 · 流转组织' }[mspId] || mspId);
const hasRole = (...roles) => Boolean(state.principal && roles.includes(state.principal.role));

function switchView(view, updateHash = true) {
  const target = document.getElementById(view);
  if (!target || !target.classList.contains('app-view')) return;
  document.querySelectorAll('.app-view').forEach(item => item.classList.toggle('active', item === target));
  document.querySelectorAll('[data-view]').forEach(item => item.classList.toggle('active', item.dataset.view === view));
  if (updateHash) history.replaceState(null, '', `#${view}`);
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

function applyOCRFields(fields) {
  const form = $('#invoice-form');
  if (!fields) return;
  if (fields.invoiceNo) form.elements.invoiceNo.value = fields.invoiceNo;
  if (fields.issueDate) form.elements.issueDate.value = fields.issueDate;
  if (fields.issuer) form.elements.issuer.value = fields.issuer;
  if (Number.isFinite(fields.amountCents) && fields.amountCents >= 0) form.elements.amount.value = (fields.amountCents / 100).toFixed(2);
  if (Number.isFinite(fields.taxCents) && fields.taxCents >= 0) form.elements.tax.value = (fields.taxCents / 100).toFixed(2);
  if (!form.elements.id.value && fields.invoiceNo) {
    const date = (fields.issueDate || new Date().toISOString().slice(0, 10)).replaceAll('-', '');
    form.elements.id.value = `INV-${date}-${fields.invoiceNo.replace(/[^A-Za-z0-9]/g, '').slice(-8)}`;
  }
  if (fields.buyerName) form.elements.buyer.value = fields.buyerName;
}

function renderOCRResult(result) {
  const node = $('#ocr-result'); const fields = result.fields || {}; const suggestions = result.suggestedFields;
  state.ocrSuggestion = suggestions || null;
  const corrections = (result.corrections || []).map(item => `<li><b>${safe(item.field)}</b>：${safe(item.from)} → ${safe(item.to)}<small>${safe(item.reason)}</small></li>`).join('');
  const warnings = (result.warnings || []).map(item => `<li>${safe(item)}</li>`).join('');
  node.hidden = false;
  node.innerHTML = `<div class="ocr-result-heading"><strong>已读取：${safe(fields.invoiceNo || '未识别发票号码')}</strong><span>${result.aiUsed ? 'OCR + AI 纠偏建议已生成' : '阿里云 OCR 识别完成'}</span></div>
    <div class="ocr-fields"><span>销售方<b>${safe(fields.issuer || '—')}</b></span><span>购买方名称<b>${safe(fields.buyerName || '—')}</b></span><span>价税合计<b>${money(fields.totalCents || 0)}</b></span></div>
    <p class="ocr-note">购买方名称已回填。新发票默认由当前创建者负责；只有需要跨组织交接时，才需在详情中选择已注册的流转员。</p>
    ${warnings ? `<div class="ocr-warning"><b>需要确认</b><ul>${warnings}</ul></div>` : ''}
    ${corrections ? `<div class="ocr-corrections"><b>AI 纠偏建议</b><ul>${corrections}</ul>${suggestions ? '<button id="apply-ocr-suggestion" type="button" class="secondary compact">应用建议并回填</button>' : ''}</div>` : (suggestions ? '<div class="ocr-corrections"><b>AI 未发现明确需要修正的字段</b></div>' : '')}`;
  $('#apply-ocr-suggestion')?.addEventListener('click', () => { applyOCRFields(state.ocrSuggestion); toast('已应用 AI 建议；请继续人工核对后再上链'); });
}

async function recognizeInvoiceFile() {
  const file = state.ocrFile;
  if (!file) return;
  const button = $('#ocr-recognize'); const resultNode = $('#ocr-result');
  button.disabled = true; button.textContent = '识别中…'; resultNode.hidden = true;
  try {
    const payload = new FormData(); payload.append('file', file, file.name);
    const result = await api('/ocr/invoice', { method: 'POST', body: payload });
    applyOCRFields(result.fields); renderOCRResult(result); toast('OCR 已完成基础回填，请人工确认后上链');
  } catch (error) { showAlert(error.message, '发票 OCR 识别失败'); }
  finally { button.disabled = !state.ocrFile; button.textContent = '开始识别'; }
}

function setOCRFile(file) {
  if (!file) return;
  state.ocrFile = file;
  $('#ocr-file-name').textContent = `${file.name} · ${(file.size / 1024 / 1024).toFixed(2)} MB`;
  $('#ocr-recognize').disabled = false;
  $('#ocr-result').hidden = true;
}

function toast(message, error = false) {
  const node = $('#toast'); node.textContent = message; node.className = `show${error ? ' error' : ''}`;
  window.clearTimeout(toast.timer); toast.timer = window.setTimeout(() => { node.className = ''; }, 3200);
}

function showAlert(message, title = '操作未完成') {
  $('#alert-title').textContent = title;
  $('#alert-message').textContent = message;
  $('#global-alert').hidden = false;
  $('#global-alert').scrollIntoView({ behavior: 'smooth', block: 'center' });
}

function showLogin() { if (!$('#login-dialog').open) $('#login-dialog').showModal(); }

function showRegister() {
  $('#login-view').hidden = true;
  $('#register-view').hidden = false;
  showLogin();
}

function showLoginView() {
  $('#register-view').hidden = true;
  $('#login-view').hidden = false;
  showLogin();
}

function applyPrincipal() {
  const principal = state.principal;
  $('#principal-name').textContent = principal ? '已登录' : '未登录';
  $('#logout').hidden = !principal;
  $('#open-register').hidden = !!principal;
  $('#create').hidden = !principal || !hasRole('ISSUER', 'PROJECT_MEMBER');
  $('#project-create-wrap').hidden = !principal || !hasRole('ISSUER', 'PROJECT_MEMBER');
  $('#reimbursement-create-wrap').hidden = !principal || !hasRole('ISSUER', 'PROJECT_MEMBER');
  $('#identity-banner').hidden = !principal;
  if (principal) {
    $('#identity-name').textContent = principal.displayName;
    $('#identity-role').textContent = roleText(principal.role);
    $('#identity-msp').textContent = principal.mspId;
  }
}

function renderHolderOptions() {
  const holders = state.users.filter(user => user.status === 'ACTIVE' && user.role === 'HOLDER');
  $('#holder-options').innerHTML = holders.map(user => `<option value="${safe(user.username)}">${safe(user.displayName)} · ${safe(user.mspId)}</option>`).join('');
}

function renderMembers() {
  const keyword = $('#member-search').value.trim().toLowerCase();
  const users = [...state.users]
    .sort((left, right) => left.mspId.localeCompare(right.mspId) || left.displayName.localeCompare(right.displayName, 'zh-CN'))
    .filter(user => !keyword || [user.displayName, user.username, user.mspId, roleText(user.role)].join(' ').toLowerCase().includes(keyword));
  $('#member-count').textContent = `共 ${users.length} 位链上成员`;
  $('#member-rows').innerHTML = users.length ? users.map(user => `<article class="member-card ${user.username === state.principal?.username ? 'is-current' : ''}">
    <div class="member-avatar">${safe(user.displayName.slice(0, 1))}</div><div class="member-main"><div class="member-name"><strong>${safe(user.displayName)}</strong>${user.username === state.principal?.username ? '<span>当前登录</span>' : ''}</div><small>@${safe(user.username)}</small></div>
    <div class="member-meta"><span>所属组织</span><b>${safe(organizationText(user.mspId))}</b></div><div class="member-meta"><span>岗位</span><b>${safe(roleText(user.role))}</b></div><span class="member-status">${user.status === 'ACTIVE' ? '在用' : safe(user.status)}</span>
  </article>`).join('') : '<p class="empty">没有符合条件的链上成员</p>';
}

async function loadUsers() {
  if (!state.principal) return;
  try { state.users = await api('/users'); renderHolderOptions(); renderMembers(); }
  catch (error) { $('#member-rows').innerHTML = `<p class="empty">${safe(error.message)}</p>`; showAlert(error.message, '链上用户读取失败'); }
}

function renderProjectOptions() {
  const available = state.projects.filter(project => ['EXECUTING', 'CLOSURE_APPROVED'].includes(project.status) && project.applicant === state.principal?.username);
  const invoiceProject = $('#invoice-project'); const reimbursementProject = $('#reimbursement-project');
  const invoiceSelected = invoiceProject.value; const reimbursementSelected = reimbursementProject.value;
  const options = available.map(project => `<option value="${safe(project.id)}">${safe(project.name)}（可用 ${money(project.availableCents)}）</option>`).join('');
  invoiceProject.innerHTML = `<option value="">不关联项目</option>${options}`;
  reimbursementProject.innerHTML = `<option value="">请选择执行中的项目</option>${options}`;
  if (available.some(project => project.id === invoiceSelected)) invoiceProject.value = invoiceSelected;
  if (available.some(project => project.id === reimbursementSelected)) reimbursementProject.value = reimbursementSelected;
  updateProjectDetailButtons();
  renderReimbursementInvoiceOptions();
}

function selectedProject(selectId) { return state.projects.find(project => project.id === $(selectId).value); }

function updateProjectDetailButtons() {
  const invoiceProject = selectedProject('#invoice-project'); const reimbursementProject = selectedProject('#reimbursement-project');
  $('#invoice-project-detail').disabled = !invoiceProject;
  $('#reimbursement-project-detail').disabled = !reimbursementProject;
}

function renderReimbursementInvoiceOptions() {
  const select = $('#reimbursement-invoice'); const projectID = $('#reimbursement-project').value;
  const selected = select.value; const usedInvoiceIDs = new Set(state.reimbursements.map(item => item.invoiceId));
  const invoices = state.invoices.filter(invoice => invoice.projectId === projectID && invoice.status !== 'VOIDED' && !usedInvoiceIDs.has(invoice.id));
  select.innerHTML = projectID ? `<option value="">请选择该项目下的发票</option>${invoices.map(invoice => `<option value="${safe(invoice.id)}">${safe(invoice.invoiceNo)} · ${safe(invoice.issuer)} · ${money(invoice.totalCents, invoice.currency)}</option>`).join('')}` : '<option value="">请先选择项目</option>';
  select.disabled = !projectID || invoices.length === 0;
  if (invoices.some(invoice => invoice.id === selected)) select.value = selected;
  $('#reimbursement-invoice-detail').disabled = !select.value;
}

function projectActions(project) {
  const buttons = [`<button class="link-button" data-project-action="view" data-project-id="${safe(project.id)}">详情</button>`];
  const ownProject = project.applicant === state.principal?.username;
  if (ownProject && ['DRAFT', 'REVISION_REQUIRED'].includes(project.status) && hasRole('ISSUER', 'PROJECT_MEMBER')) {
    buttons.push(`<button class="link-button" data-project-action="edit" data-project-id="${safe(project.id)}">编辑</button><button class="link-button" data-project-action="submit" data-project-id="${safe(project.id)}">提交</button>`);
  }
  if (ownProject && project.status === 'EXECUTING' && hasRole('ISSUER', 'PROJECT_MEMBER')) buttons.push(`<button class="link-button" data-project-action="closure" data-project-id="${safe(project.id)}">申请结项</button>`);
  if (project.status === 'PENDING_REVIEW' && hasRole('PROJECT_REVIEWER')) buttons.push(`<button class="link-button" data-project-action="review" data-project-id="${safe(project.id)}">立项审核</button>`);
  if (project.status === 'CLOSURE_REVIEW' && hasRole('PROJECT_REVIEWER')) buttons.push(`<button class="link-button" data-project-action="closure-review" data-project-id="${safe(project.id)}">结项审核</button>`);
  return buttons.join(' ');
}

function renderProjects() {
  $('#project-rows').innerHTML = state.projects.length ? state.projects.map(project => `<tr>
    <td><strong>${safe(project.name)}</strong><small>${safe(project.content.slice(0, 36))}</small></td>
    <td><strong>${money(project.budgetCents)}</strong><small>可用 ${money(project.availableCents)} · 冻结 ${money(project.reservedCents)} · 已支付 ${money(project.paidCents)}</small></td>
    <td>${safe(project.applicant)}<small>${safe(project.applicantMspId)}</small></td><td>${safe(project.expectedEndDate)}</td>
    <td><span class="status ${project.status === 'REVISION_REQUIRED' ? 'voided' : project.status.includes('REVIEW') ? 'circulating' : ''}">${projectStatusText(project.status)}</span></td>
    <td class="project-actions">${projectActions(project)}</td></tr>`).join('') : '<tr><td colspan="6" class="empty">暂无链上项目</td></tr>';
  renderProjectOptions();
}

async function loadProjects() {
  if (!state.principal) return;
  try { state.projects = await api('/projects'); renderProjects(); renderReimbursements(); }
  catch (error) { $('#project-rows').innerHTML = `<tr><td colspan="6" class="empty">${safe(error.message)}</td></tr>`; showAlert(error.message, '项目读取失败'); }
}

function reimbursementActions(reimbursement) {
  const buttons = [];
  if (reimbursement.status === 'PENDING_REVIEW' && hasRole('PROJECT_REVIEWER')) buttons.push(`<button class="link-button" data-reimbursement-action="review" data-reimbursement-id="${safe(reimbursement.id)}">审核</button>`);
  if (reimbursement.status === 'APPROVED_RESERVED' && hasRole('FINANCE_ADMIN')) buttons.push(`<button class="link-button" data-reimbursement-action="pay" data-reimbursement-id="${safe(reimbursement.id)}">确认支付</button>`);
  return buttons.join(' ') || '—';
}

function renderReimbursements() {
  $('#reimbursement-rows').innerHTML = state.reimbursements.length ? state.reimbursements.map(item => `<tr>
    <td><strong>报销申请</strong><small>${safe(item.createdAt)}</small></td><td>${safe(state.projects.find(project => project.id === item.projectId)?.name || '项目') }<small>发票：${safe(state.invoices.find(invoice => invoice.id === item.invoiceId)?.invoiceNo || '已关联发票')}</small></td>
    <td>${money(item.amountCents)}</td><td>${safe(item.applicant)}</td><td><span class="status ${item.status === 'REVISION_REQUIRED' ? 'voided' : item.status === 'PENDING_REVIEW' ? 'circulating' : ''}">${reimbursementStatusText(item.status)}</span></td><td>${reimbursementActions(item)}</td></tr>`).join('') : '<tr><td colspan="6" class="empty">暂无链上报销单</td></tr>';
}

async function loadReimbursements() {
  if (!state.principal) return;
  try { state.reimbursements = await api('/reimbursements'); renderReimbursements(); renderReimbursementInvoiceOptions(); }
  catch (error) { $('#reimbursement-rows').innerHTML = `<tr><td colspan="6" class="empty">${safe(error.message)}</td></tr>`; showAlert(error.message, '报销单读取失败'); }
}

async function reloadBusinessData() { await Promise.all([loadUsers(), loadInvoices(), loadProjects(), loadReimbursements()]); }

function updateMetrics() {
  const invoices = state.invoices;
  $('#metric-total').textContent = invoices.length;
  $('#metric-circulating').textContent = invoices.filter(item => item.status === 'IN_CIRCULATION').length;
  const cnyTotal = invoices.filter(item => item.currency === 'CNY' && item.status !== 'VOIDED').reduce((sum, item) => sum + item.totalCents, 0);
  $('#metric-amount').textContent = money(cnyTotal);
}

function renderInvoices() {
  const keyword = $('#search').value.trim().toLowerCase();
  const list = state.invoices.filter(item => !keyword || [item.id, item.invoiceNo, item.issuer, item.buyer, item.currentHolder].join(' ').toLowerCase().includes(keyword));
  $('#invoice-count').textContent = `共 ${list.length} 条记录`;
  $('#invoice-rows').innerHTML = list.length ? list.map(item => `<tr>
    <td><strong>${safe(item.invoiceNo)}</strong><small>${safe(item.id)} · ${safe(item.issueDate)}</small></td>
    <td><strong>${safe(item.issuer)}</strong><small>→ ${safe(item.buyer)}</small></td>
    <td>${money(item.totalCents, item.currency)}</td><td>${safe(item.currentHolder)}<small>${safe(item.holderMspId || '旧版记录')}</small></td>
    <td><span class="status ${item.status === 'IN_CIRCULATION' ? 'circulating' : item.status === 'VOIDED' ? 'voided' : ''}">${statusText(item.status)}</span></td>
    <td><button class="link-button" data-detail="${safe(item.id)}">详情</button></td></tr>`).join('') : '<tr><td colspan="6" class="empty">没有符合条件的发票记录</td></tr>';
}

async function loadInvoices() {
  if (!state.principal) return;
  try { state.invoices = await api('/invoices'); updateMetrics(); renderInvoices(); renderReimbursementInvoiceOptions(); renderReimbursements(); }
  catch (error) { $('#invoice-rows').innerHTML = `<tr><td colspan="6" class="empty">${safe(error.message)}</td></tr>`; showAlert(error.message, '账本读取失败'); }
}

function transferForm(invoice) {
  const allowed = hasRole('ISSUER', 'HOLDER', 'PROJECT_MEMBER') && invoice.status !== 'VOIDED' && invoice.currentHolder === state.principal.username && invoice.holderMspId === state.principal.mspId;
  if (!allowed) return '<p class="detail-sub">该发票当前无需你处理。只有当前责任人可在确有跨组织交接需要时发起流转。</p>';
  return `<form id="transfer-form" class="transfer-form"><label>下一位流转员账号<input name="to" list="holder-options" required placeholder="选择已注册的跨组织流转员"></label><label>接收组织<select name="toMspId"><option value="Org1MSP">Org1MSP</option><option value="Org2MSP">Org2MSP</option></select></label><div><small>这是可选的跨组织交接，不影响普通项目报销。接收方必须是链上已注册的流转员。</small></div><button type="submit">确认跨组织流转</button></form>`;
}

function voidForm(invoice) {
  const allowed = state.principal.role === 'ISSUER' && invoice.status !== 'VOIDED' && invoice.issuerMspId === state.principal.mspId;
  if (!allowed) return invoice.status === 'VOIDED' ? `<p class="detail-sub">已作废：${safe(invoice.voidReason || '未填写原因')}</p>` : '';
  return `<form id="void-form" class="transfer-form"><label>作废原因<input name="reason" required minlength="2" placeholder="例如：开票信息录入错误"></label><div><small>作废不会删除原记录，会新增一笔链上作废交易。</small></div><div></div><button class="danger" type="submit">确认作废</button></form>`;
}

async function openDetail(id) {
  try {
    const [invoice, flows, history] = await Promise.all([api(`/invoices/${encodeURIComponent(id)}`), api(`/invoices/${encodeURIComponent(id)}/flows`), api(`/invoices/${encodeURIComponent(id)}/history`)]);
    $('#detail-content').innerHTML = `<p class="eyebrow">ON-CHAIN INVOICE</p><h2 class="detail-title">${safe(invoice.invoiceNo)}</h2><p class="detail-sub">${safe(invoice.id)} · ${safe(statusText(invoice.status))}</p>
      <div class="details"><div><span>销售方</span><b>${safe(invoice.issuer)}</b></div><div><span>购买方</span><b>${safe(invoice.buyer)}</b></div><div><span>当前持有人</span><b>${safe(invoice.currentHolder)}</b></div><div><span>发行组织</span><b>${safe(invoice.issuerMspId || '旧版记录')}</b></div><div><span>持有人组织</span><b>${safe(invoice.holderMspId || '旧版记录')}</b></div><div><span>价税合计</span><b>${money(invoice.totalCents, invoice.currency)}</b></div></div>
      <div class="detail-section"><h3>内容指纹</h3><p class="hash">${safe(invoice.dataHash)}</p><button class="secondary" data-fill-verify="${safe(invoice.id)}" data-hash="${safe(invoice.dataHash)}">填入核验中心</button></div>
      <div class="detail-section"><h3>发票流转</h3><div class="timeline">${flows.map(flow => `<div><b>${flow.type === 'ISSUE' ? '开具存证' : flow.type === 'VOID' ? '发票作废' : '流转上链'}：${safe(flow.from)} → ${safe(flow.to)}</b><small>${safe(flow.timestamp)} · 签名组织：${safe(flow.operator)}</small></div>`).join('') || '<p>暂无流转记录</p>'}</div></div>
      <div class="detail-section"><h3>执行流转</h3>${transferForm(invoice)}</div>
      <div class="detail-section"><h3>作废 / 红冲</h3>${voidForm(invoice)}</div>
      <div class="detail-section"><h3>链上历史（${history.length} 笔）</h3><div class="timeline">${history.map(record => `<div><b>${safe(record.txId.slice(0, 22))}…</b><small>${safe(record.timestamp)} · ${record.isDelete ? '删除' : `状态：${safe(record.value?.status || '')}`}</small></div>`).join('') || '<p>暂无历史记录</p>'}</div></div>`;
    $('#detail-dialog').showModal();
    $('#transfer-form')?.addEventListener('submit', event => submitTransfer(event, invoice.id));
    $('#void-form')?.addEventListener('submit', event => submitVoid(event, invoice.id));
    document.querySelector('[data-fill-verify]').addEventListener('click', event => { $('#verify-id').value = event.currentTarget.dataset.fillVerify; $('#verify-hash').value = event.currentTarget.dataset.hash; $('#detail-dialog').close(); switchView('verify'); });
  } catch (error) { showAlert(error.message, '发票详情读取失败'); }
}

async function submitTransfer(event, id) {
  event.preventDefault();
  try { await api(`/invoices/${encodeURIComponent(id)}/transfers`, { method: 'POST', body: JSON.stringify(Object.fromEntries(new FormData(event.currentTarget))) }); toast('跨组织流转已由当前责任人确认并上链'); $('#detail-dialog').close(); await loadInvoices(); }
  catch (error) { showAlert(error.message, '流转失败'); }
}

async function submitVoid(event, id) {
  event.preventDefault();
  try { await api(`/invoices/${encodeURIComponent(id)}/void`, { method: 'POST', body: JSON.stringify(Object.fromEntries(new FormData(event.currentTarget))) }); toast('发票已作废，原始记录和历史均已保留'); $('#detail-dialog').close(); await loadInvoices(); }
  catch (error) { showAlert(error.message, '作废失败'); }
}

async function openProjectDetail(id) {
  try {
    const project = state.projects.find(item => item.id === id);
    const events = await api(`/projects/${encodeURIComponent(id)}/events`);
    $('#detail-content').innerHTML = `<p class="eyebrow">ON-CHAIN PROJECT</p><h2 class="detail-title">${safe(project?.name || '项目详情')}</h2><p class="detail-sub">${safe(projectStatusText(project?.status))}</p>
      <div class="details"><div><span>申请人</span><b>${safe(project?.applicant)}</b></div><div><span>预算</span><b>${money(project?.budgetCents || 0)}</b></div><div><span>可用余额</span><b>${money(project?.availableCents || 0)}</b></div><div><span>冻结额度</span><b>${money(project?.reservedCents || 0)}</b></div><div><span>已支付</span><b>${money(project?.paidCents || 0)}</b></div><div><span>预期结项</span><b>${safe(project?.expectedEndDate)}</b></div></div>
      <div class="detail-section"><h3>项目内容</h3><p>${safe(project?.content)}</p></div><div class="detail-section"><h3>最近审核意见</h3><p>${safe(project?.reviewOpinion || '暂无')}</p></div>
      <div class="detail-section"><h3>链上项目事件</h3><div class="timeline">${events.map(item => `<div><b>${safe(item.type)} · ${safe(item.actor)}</b><small>${safe(item.timestamp)}${item.note ? ` · ${safe(item.note)}` : ''}</small></div>`).join('') || '<p>暂无事件</p>'}</div></div>
      ${projectReviewForm(project)}`;
    $('#detail-dialog').showModal();
    $('#project-review-form')?.addEventListener('submit', event => submitProjectReview(event, project.id));
  } catch (error) { showAlert(error.message, '项目详情读取失败'); }
}

function projectReviewForm(project) {
  if (!project || !hasRole('PROJECT_REVIEWER') || !['PENDING_REVIEW', 'CLOSURE_REVIEW'].includes(project.status)) return '';
  const closure = project.status === 'CLOSURE_REVIEW';
  return `<div class="detail-section review-section"><h3>${closure ? '结项审核' : '立项审核'}</h3><p class="detail-sub">请填写审核意见，再选择通过或要求项目组修改。</p><form id="project-review-form" class="review-form" data-endpoint="${closure ? 'closure-review' : 'review'}"><label>审核意见<textarea name="opinion" required minlength="2" maxlength="1000" placeholder="例如：项目目标清晰，同意立项；或请补充预算依据。"></textarea></label><div><button type="submit" name="decision" value="APPROVE">${closure ? '通过结项' : '通过立项'}</button><button type="submit" name="decision" value="REVISION" class="secondary">要求修改</button></div></form></div>`;
}

async function submitProjectReview(event, id) {
  event.preventDefault();
  const decision = event.submitter?.value;
  const opinion = new FormData(event.currentTarget).get('opinion')?.trim();
  if (!decision || !opinion) return;
  const endpoint = event.currentTarget.dataset.endpoint;
  try {
    await api(`/projects/${encodeURIComponent(id)}/${endpoint}`, { method: 'POST', body: JSON.stringify({ decision, opinion }) });
    $('#detail-dialog').close(); toast(decision === 'APPROVE' ? '审核通过，结果已写入链上' : '已要求项目组修改，意见已写入链上'); await reloadBusinessData();
  } catch (error) { showAlert(error.message, '项目审核失败'); }
}

function editProject(id) {
  const project = state.projects.find(item => item.id === id);
  if (!project) return;
  state.editingProjectId = id;
  const form = $('#project-form');
  form.elements.id.value = project.id; form.elements.id.disabled = true; form.elements.name.value = project.name; form.elements.content.value = project.content;
  form.elements.expectedEndDate.value = project.expectedEndDate; form.elements.budget.value = (project.budgetCents / 100).toFixed(2);
  $('#project-submit-label').textContent = '保存修改后的项目草稿';
  switchView('projects');
}

async function handleProjectAction(event) {
  const button = event.target.closest('[data-project-action]');
  if (!button) return;
  const { projectAction: action, projectId: id } = button.dataset;
  if (action === 'view') { openProjectDetail(id); return; }
  if (action === 'edit') { editProject(id); return; }
  try {
    if (action === 'submit') await api(`/projects/${encodeURIComponent(id)}/submit`, { method: 'POST' });
    if (action === 'closure') {
      const materials = window.prompt('填写结项材料说明（例如：结项报告、成果照片、演示视频链接）：');
      if (materials === null) return;
      await api(`/projects/${encodeURIComponent(id)}/closure`, { method: 'POST', body: JSON.stringify({ materials }) });
    }
    if (action === 'review' || action === 'closure-review') { await openProjectDetail(id); return; }
    toast(action === 'submit' ? '项目申请已提交审核' : '结项申请已提交审核'); await reloadBusinessData();
  } catch (error) { showAlert(error.message, '项目操作失败'); }
}

async function handleReimbursementAction(event) {
  const button = event.target.closest('[data-reimbursement-action]');
  if (!button) return;
  const { reimbursementAction: action, reimbursementId: id } = button.dataset;
  try {
    if (action === 'review') {
      const approved = window.confirm('点击“确定”为通过并冻结项目额度；点击“取消”为要求修改。');
      const opinion = window.prompt('请输入审核意见（必填）：');
      if (opinion === null) return;
      await api(`/reimbursements/${encodeURIComponent(id)}/review`, { method: 'POST', body: JSON.stringify({ decision: approved ? 'APPROVE' : 'REVISION', opinion }) });
      toast(approved ? '报销已审核通过，项目额度已冻结' : '报销单已退回修改');
    }
    if (action === 'pay') {
      if (!window.confirm('确认已完成实际拨款提现吗？此操作将把冻结额度转为已支付。')) return;
      await api(`/reimbursements/${encodeURIComponent(id)}/pay`, { method: 'POST' });
      toast('报销款已支付，资金池已更新');
    }
    await reloadBusinessData();
  } catch (error) { showAlert(error.message, action === 'pay' ? '支付失败' : '报销审核失败'); }
}

$('#project-form').addEventListener('submit', async event => {
  event.preventDefault();
  const form = event.currentTarget; const payload = Object.fromEntries(new FormData(form));
  payload.budgetCents = cents(payload.budget); delete payload.budget;
  $('#project-result').hidden = true;
  try {
    if (state.editingProjectId) {
      await api(`/projects/${encodeURIComponent(state.editingProjectId)}`, { method: 'PUT', body: JSON.stringify(payload) });
      toast('项目草稿已更新，请重新提交审核');
    } else {
      await api('/projects', { method: 'POST', body: JSON.stringify(payload) });
      toast('项目草稿已写入链上，请点击“提交”发起审核');
    }
    const node = $('#project-result'); node.hidden = false; node.textContent = state.editingProjectId ? '草稿已更新，可再次提交项目审核。' : '项目草稿创建成功，请在项目列表中点击“提交”。';
    state.editingProjectId = null; form.reset(); form.elements.id.disabled = false; $('#project-submit-label').textContent = '保存项目草稿'; await loadProjects();
  } catch (error) { showAlert(error.message, '项目保存失败'); }
});

$('#reimbursement-form').addEventListener('submit', async event => {
  event.preventDefault();
  const form = event.currentTarget; const payload = Object.fromEntries(new FormData(form));
  $('#reimbursement-result').hidden = true;
  try {
    await api('/reimbursements', { method: 'POST', body: JSON.stringify(payload) });
    const node = $('#reimbursement-result'); node.hidden = false; node.textContent = '报销单已提交审核，请等待项目管理审核员处理。';
    form.reset(); toast('报销单已提交项目管理审核'); await loadReimbursements();
  } catch (error) { showAlert(error.message, '报销提交失败'); }
});

$('#invoice-form').addEventListener('submit', async event => {
  event.preventDefault(); const formElement = event.currentTarget; const form = new FormData(formElement); const payload = Object.fromEntries(form); payload.amountCents = cents(payload.amount); payload.taxCents = cents(payload.tax); delete payload.amount; delete payload.tax;
  $('#create-result').hidden = true;
  try { await api('/invoices', { method: 'POST', body: JSON.stringify(payload) }); const node = $('#create-result'); node.hidden = false; node.textContent = '存证成功，发票已写入可信账本。'; toast('发票已完成链上存证'); formElement.reset(); $('#invoice-form [name="issueDate"]').value = new Date().toISOString().slice(0, 10); await Promise.all([loadInvoices(), loadReimbursements()]); }
  catch (error) { showAlert(error.message, '存证失败'); }
});

$('#verify-form').addEventListener('submit', async event => {
  event.preventDefault(); const id = $('#verify-id').value.trim(); const hash = $('#verify-hash').value.trim(); const node = $('#verify-result');
  try { const result = await api(`/invoices/${encodeURIComponent(id)}/verify`, { method: 'POST', body: JSON.stringify({ dataHash: hash }) }); node.className = `verify-result ${result.valid ? 'success' : 'failure'}`; node.innerHTML = result.valid ? `✓ 核验通过：发票 <strong>${safe(result.invoice.invoiceNo)}</strong> 的展示内容与链上存证一致。当前持有人：${safe(result.invoice.currentHolder)}。` : `× 核验未通过：${safe(result.reason)}。请检查发票内容或内容指纹。`; }
  catch (error) { node.className = 'verify-result failure'; node.textContent = error.message; showAlert(error.message, '核验失败'); }
});

$('#login-form').addEventListener('submit', async event => {
  event.preventDefault();
  const errorNode = $('#login-error'); errorNode.hidden = true;
  try { state.principal = await api('/auth/login', { method: 'POST', body: JSON.stringify(Object.fromEntries(new FormData(event.currentTarget))) }); $('#login-dialog').close(); applyPrincipal(); toast(`已登录：${state.principal.displayName}，Fabric 身份 ${state.principal.mspId}`); await reloadBusinessData(); }
  catch (error) { errorNode.textContent = error.message; errorNode.hidden = false; }
});

$('#register-form').addEventListener('submit', async event => {
  event.preventDefault();
  const formElement = event.currentTarget;
  const errorNode = $('#register-error'); errorNode.hidden = true;
  const payload = Object.fromEntries(new FormData(formElement));
  if (payload.password !== payload.confirmPassword) {
    errorNode.textContent = '两次输入的密码不一致'; errorNode.hidden = false; return;
  }
  delete payload.confirmPassword;
  try {
    state.principal = await api('/auth/register', { method: 'POST', body: JSON.stringify(payload) });
    formElement.reset(); $('#login-dialog').close(); applyPrincipal();
    toast(`已注册 ${state.principal.displayName}，业务档案已写入 Fabric`);
    await reloadBusinessData();
  } catch (error) { errorNode.textContent = error.message; errorNode.hidden = false; }
});

$('#logout').addEventListener('click', async () => { await api('/auth/logout', { method: 'POST' }); state.principal = null; state.invoices = []; state.users = []; state.projects = []; state.reimbursements = []; applyPrincipal(); $('#invoice-rows').innerHTML = '<tr><td colspan="6" class="empty">请登录后查看账本</td></tr>'; $('#project-rows').innerHTML = '<tr><td colspan="6" class="empty">请登录后查看项目</td></tr>'; $('#reimbursement-rows').innerHTML = '<tr><td colspan="6" class="empty">请登录后查看报销单</td></tr>'; $('#member-rows').innerHTML = '<p class="empty">请登录后查看链上成员名录</p>'; showLoginView(); });
$('#search').addEventListener('input', renderInvoices);
$('#member-search').addEventListener('input', renderMembers);
$('#ocr-file').addEventListener('change', event => { if (event.currentTarget.files[0]) setOCRFile(event.currentTarget.files[0]); });
['dragenter', 'dragover'].forEach(type => $('#ocr-drop-zone').addEventListener(type, event => { event.preventDefault(); event.stopPropagation(); $('#ocr-drop-zone').classList.add('dragging'); }));
['dragleave', 'drop'].forEach(type => $('#ocr-drop-zone').addEventListener(type, event => { event.preventDefault(); event.stopPropagation(); $('#ocr-drop-zone').classList.remove('dragging'); }));
$('#ocr-drop-zone').addEventListener('drop', event => {
  const file = event.dataTransfer?.files?.[0];
  if (file) setOCRFile(file);
});
$('#ocr-recognize').addEventListener('click', recognizeInvoiceFile);
$('#refresh').addEventListener('click', loadInvoices);
$('#refresh-members').addEventListener('click', loadUsers);
$('#refresh-projects').addEventListener('click', loadProjects);
$('#refresh-reimbursements').addEventListener('click', loadReimbursements);
$('#invoice-rows').addEventListener('click', event => { const id = event.target.dataset.detail; if (id) openDetail(id); });
$('#project-rows').addEventListener('click', handleProjectAction);
$('#reimbursement-rows').addEventListener('click', handleReimbursementAction);
$('#invoice-project').addEventListener('change', updateProjectDetailButtons);
$('#reimbursement-project').addEventListener('change', () => { updateProjectDetailButtons(); renderReimbursementInvoiceOptions(); });
$('#reimbursement-invoice').addEventListener('change', () => { $('#reimbursement-invoice-detail').disabled = !$('#reimbursement-invoice').value; });
$('#invoice-project-detail').addEventListener('click', () => { const project = selectedProject('#invoice-project'); if (project) openProjectDetail(project.id); });
$('#reimbursement-project-detail').addEventListener('click', () => { const project = selectedProject('#reimbursement-project'); if (project) openProjectDetail(project.id); });
$('#reimbursement-invoice-detail').addEventListener('click', () => { const id = $('#reimbursement-invoice').value; if (id) openDetail(id); });
$('#close-dialog').addEventListener('click', () => $('#detail-dialog').close());
$('#close-alert').addEventListener('click', () => { $('#global-alert').hidden = true; });
document.addEventListener('pointerdown', event => { const alert = $('#global-alert'); if (!alert.hidden && !alert.contains(event.target)) alert.hidden = true; });
$('#open-register').addEventListener('click', showRegister);
$('#show-register').addEventListener('click', showRegister);
$('#show-login').addEventListener('click', showLoginView);
document.querySelectorAll('[data-view]').forEach(button => button.addEventListener('click', () => switchView(button.dataset.view)));
document.querySelector('[data-view-link="dashboard"]').addEventListener('click', event => { event.preventDefault(); switchView('dashboard'); });
window.addEventListener('hashchange', () => switchView(location.hash.slice(1) || 'dashboard', false));
$('#invoice-form [name="issueDate"]').value = new Date().toISOString().slice(0, 10);

switchView(location.hash.slice(1) || 'dashboard', false);
(async () => { try { state.principal = await api('/auth/me'); applyPrincipal(); await reloadBusinessData(); } catch (_) { applyPrincipal(); showLoginView(); } })();
