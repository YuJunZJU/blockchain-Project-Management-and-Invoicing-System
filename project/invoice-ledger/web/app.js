const state = { invoices: [], organizations: [], registrationOrganizations: [], principal: null, projects: [], reimbursements: [], users: [], editingProjectId: null, ocrFile: null, ocrSuggestion: null };
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
const displayTime = (value) => {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'medium', hour12: false, timeZone: 'Asia/Shanghai' }).format(date);
};
const statusText = (status) => ({ ISSUED: '已开具', IN_CIRCULATION: '流转中', VOIDED: '已作废' }[status] || status);
const voidRequestStatusText = (status) => ({ PENDING_REVIEW: '待开票员审核', APPROVED: '已批准并作废', REJECTED: '已驳回' }[status] || status);
const projectStatusText = (status) => ({ DRAFT: '草稿', PENDING_REVIEW: '待立项审核', REVISION_REQUIRED: '需修改', EXECUTING: '执行中', CLOSURE_REVIEW: '待结项审核', CLOSURE_APPROVED: '结项验收通过' }[status] || status);
const reimbursementStatusText = (status) => ({ PENDING_REVIEW: '待报销审核', REVISION_REQUIRED: '材料需修改', APPROVED_RESERVED: '已审核，额度已冻结', PAID: '已支付' }[status] || status);
const roleText = (role) => ({ ISSUER: '开票员', HOLDER: '跨组织流转员', AUDITOR: '审计员', PROJECT_MEMBER: '项目组成员', PROJECT_REVIEWER: '项目管理审核员', FINANCE_ADMIN: '财务管理员', ORG_ADMIN: '组织管理员' }[role] || role);
const organizationText = (mspId) => ({ Org1MSP: 'Org1 · Fabric 节点 1', Org2MSP: 'Org2 · Fabric 节点 2' }[mspId] || mspId);
const organizationTypeText = (type) => ({ PRIMARY: '主体组织', PROJECT_TEAM: '公司内部项目组', EXTERNAL: '外部协作组织' }[type] || type);
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
  loadRegistrationOrganizations();
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
  $('#organization-create-wrap').hidden = !principal || !hasRole('ORG_ADMIN');
  $('#organization-permission-note').hidden = !!principal && hasRole('ORG_ADMIN');
  $('#identity-banner').hidden = !principal;
  if (principal) {
    $('#identity-name').textContent = principal.displayName;
    $('#identity-role').textContent = roleText(principal.role);
    const organization = state.organizations.find(item => item.id === principal.organizationId) || state.registrationOrganizations.find(item => item.id === principal.organizationId);
    $('#identity-msp').textContent = organization ? `${organization.name} · ${principal.mspId}` : principal.mspId;
  }
}

function renderHolderOptions() {
  const holders = state.users.filter(user => user.status === 'ACTIVE' && user.role === 'HOLDER');
  $('#holder-options').innerHTML = holders.map(user => `<option value="${safe(user.username)}">${safe(user.displayName)} · ${safe(user.mspId)}</option>`).join('');
}

async function loadUsers() {
  if (!state.principal) return;
  try { state.users = await api('/users'); renderHolderOptions(); renderOrganizations(); }
  catch (error) { showAlert(error.message, '链上用户读取失败'); }
}

function updateOrganizationParentOptions() {
  const isProjectTeam = $('#organization-type').value === 'PROJECT_TEAM';
  const wrap = $('#organization-parent-wrap'); const select = $('#organization-parent');
  wrap.hidden = !isProjectTeam;
  if (!isProjectTeam) { select.value = ''; return; }
  const primaryOrganizations = state.organizations.filter(item => item.type === 'PRIMARY' && item.status === 'ACTIVE');
  const selected = select.value;
  select.innerHTML = primaryOrganizations.length
    ? `<option value="">请选择主体组织</option>${primaryOrganizations.map(item => `<option value="${safe(item.id)}">${safe(item.name)}</option>`).join('')}`
    : '<option value="">请先登记主体组织</option>';
  select.disabled = primaryOrganizations.length === 0;
  if (primaryOrganizations.some(item => item.id === selected)) select.value = selected;
}

function renderOrganizations() {
  const organizations = [...state.organizations].sort((left, right) => left.type.localeCompare(right.type) || left.name.localeCompare(right.name, 'zh-CN'));
  $('#organization-rows').innerHTML = organizations.length ? organizations.map(item => {
    const parent = state.organizations.find(candidate => candidate.id === item.parentId);
    const memberCount = state.users.filter(user => user.organizationId === item.id && user.status === 'ACTIVE').length;
    return `<article class="organization-card" data-organization-detail="${safe(item.id)}"><div class="organization-card-heading"><span class="organization-type ${safe(item.type.toLowerCase())}">${safe(organizationTypeText(item.type))}</span><span class="member-status">${item.status === 'ACTIVE' ? '在用' : safe(item.status)}</span></div><h3>${safe(item.name)}</h3><p>${safe(item.description || '暂无组织说明')}</p><dl><div><dt>上级主体</dt><dd>${safe(parent?.name || '—')}</dd></div><div><dt>链上接入节点</dt><dd>${safe(organizationText(item.mspId))}</dd></div><div><dt>登记人</dt><dd>@${safe(item.createdBy)}</dd></div></dl><button class="secondary organization-members-button" type="button" data-organization-detail="${safe(item.id)}">查看成员（${memberCount}）</button></article>`;
  }).join('') : '<p class="empty">暂无链上业务组织。请先由组织管理员登记主体组织。</p>';
  updateOrganizationParentOptions();
}

async function loadOrganizations() {
  if (!state.principal) return;
  try { state.organizations = await api('/organizations'); renderOrganizations(); }
  catch (error) { $('#organization-rows').innerHTML = `<p class="empty">${safe(error.message)}</p>`; showAlert(error.message, '组织目录读取失败'); }
}

async function loadRegistrationOrganizations() {
  const select = $('#registration-organization'); const hint = $('#registration-organization-hint'); const submit = $('#register-submit');
  select.disabled = true; submit.disabled = true;
  select.innerHTML = '<option value="">正在读取组织目录…</option>';
  try {
    state.registrationOrganizations = await api('/auth/organizations');
    const organizations = state.registrationOrganizations.filter(item => item.status === 'ACTIVE');
    select.innerHTML = organizations.length
      ? `<option value="">请选择所属业务组织</option>${organizations.map(item => `<option value="${safe(item.id)}">${safe(item.name)} · ${safe(organizationTypeText(item.type))}</option>`).join('')}`
      : '<option value="">尚未登记业务组织</option>';
    select.disabled = organizations.length === 0;
    submit.disabled = organizations.length === 0;
    hint.hidden = organizations.length > 0;
  } catch (error) {
    select.innerHTML = '<option value="">组织目录读取失败</option>';
    hint.hidden = false; hint.textContent = error.message;
  }
}

function openOrganizationDetail(id) {
  const organization = state.organizations.find(item => item.id === id);
  if (!organization) return;
  const parent = state.organizations.find(item => item.id === organization.parentId);
  const members = state.users
    .filter(user => user.organizationId === organization.id)
    .sort((left, right) => left.displayName.localeCompare(right.displayName, 'zh-CN'));
  $('#detail-content').innerHTML = `<p class="eyebrow">ON-CHAIN BUSINESS ORGANIZATION</p><h2 class="detail-title">${safe(organization.name)}</h2><p class="detail-sub">${safe(organizationTypeText(organization.type))} · ${safe(organization.status === 'ACTIVE' ? '在用' : organization.status)}</p>
    <div class="details"><div><span>上级主体</span><b>${safe(parent?.name || '—')}</b></div><div><span>账本接入节点</span><b>${safe(organizationText(organization.mspId))}</b></div><div><span>登记人</span><b>@${safe(organization.createdBy)}</b></div><div><span>登记时间</span><b>${safe(displayTime(organization.createdAt))}</b></div></div>
    <div class="detail-section"><h3>组织说明</h3><p>${safe(organization.description || '暂无组织说明')}</p></div>
    <div class="detail-section"><h3>组织成员（${members.length} 位）</h3>${members.length ? `<div class="organization-member-list">${members.map(user => `<div><span class="member-avatar">${safe(user.displayName.slice(0, 1))}</span><p><b>${safe(user.displayName)}</b><small>@${safe(user.username)} · ${safe(roleText(user.role))}</small></p><em>${safe(organizationText(user.mspId))}</em></div>`).join('')}</div>` : '<p class="detail-sub">该组织暂未绑定业务用户。请在注册页选择此组织后注册成员。</p>'}</div>`;
  $('#detail-dialog').showModal();
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
  const buttons = [`<button class="link-button" data-reimbursement-action="view" data-reimbursement-id="${safe(reimbursement.id)}">详情</button>`];
  if (reimbursement.status === 'PENDING_REVIEW' && hasRole('PROJECT_REVIEWER')) buttons.push(`<button class="link-button" data-reimbursement-action="review" data-reimbursement-id="${safe(reimbursement.id)}">审核</button>`);
  if (reimbursement.status === 'APPROVED_RESERVED' && hasRole('FINANCE_ADMIN')) buttons.push(`<button class="link-button" data-reimbursement-action="pay" data-reimbursement-id="${safe(reimbursement.id)}">确认支付</button>`);
  return buttons.join(' ');
}

function renderReimbursements() {
  $('#reimbursement-rows').innerHTML = state.reimbursements.length ? state.reimbursements.map(item => `<tr>
    <td><strong>报销申请</strong><small>${safe(displayTime(item.createdAt))}</small></td><td>${safe(state.projects.find(project => project.id === item.projectId)?.name || '项目') }<small>发票：${safe(state.invoices.find(invoice => invoice.id === item.invoiceId)?.invoiceNo || '已关联发票')}</small></td>
    <td>${money(item.amountCents)}</td><td>${safe(item.applicant)}</td><td><span class="status ${item.status === 'REVISION_REQUIRED' ? 'voided' : item.status === 'PENDING_REVIEW' ? 'circulating' : ''}">${reimbursementStatusText(item.status)}</span></td><td>${reimbursementActions(item)}</td></tr>`).join('') : '<tr><td colspan="6" class="empty">暂无链上报销单</td></tr>';
}

async function loadReimbursements() {
  if (!state.principal) return;
  try { state.reimbursements = await api('/reimbursements'); renderReimbursements(); renderReimbursementInvoiceOptions(); }
  catch (error) { $('#reimbursement-rows').innerHTML = `<tr><td colspan="6" class="empty">${safe(error.message)}</td></tr>`; showAlert(error.message, '报销单读取失败'); }
}

async function reloadBusinessData() { await Promise.all([loadUsers(), loadOrganizations(), loadInvoices(), loadProjects(), loadReimbursements()]); }

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
  return `<form id="transfer-form" class="transfer-form"><label>下一位流转员<input name="to" autocomplete="off" required placeholder="输入账号或姓名搜索成员"></label><label>接收组织<select name="toMspId"><option value="Org1MSP">Org1MSP</option><option value="Org2MSP">Org2MSP</option></select></label><div><small>输入姓名或账号后，从下方候选成员中点击选择；系统会自动匹配所属组织。</small></div><button type="submit">确认跨组织流转</button><div id="holder-suggestions" class="holder-suggestions" hidden></div></form>`;
}

function bindHolderPicker(form) {
  const input = form.elements.to; const mspSelect = form.elements.toMspId; const suggestions = $('#holder-suggestions');
  const holders = state.users.filter(user => user.status === 'ACTIVE' && user.role === 'HOLDER');
  const render = () => {
    const query = input.value.trim().toLowerCase();
    const matches = holders.filter(user => !query || [user.username, user.displayName, user.mspId].some(value => value.toLowerCase().includes(query))).slice(0, 6);
    suggestions.hidden = matches.length === 0;
    suggestions.innerHTML = matches.map(user => `<button type="button" class="holder-suggestion" data-holder="${safe(user.username)}" data-holder-msp="${safe(user.mspId)}"><span><b>${safe(user.displayName)}</b> · @${safe(user.username)}</span><small>${safe(organizationText(user.mspId))}</small></button>`).join('');
    const exact = holders.find(user => user.username.toLowerCase() === query);
    if (exact) mspSelect.value = exact.mspId;
  };
  input.addEventListener('focus', render);
  input.addEventListener('input', render);
  suggestions.addEventListener('click', event => {
    const button = event.target.closest('[data-holder]');
    if (!button) return;
    input.value = button.dataset.holder; mspSelect.value = button.dataset.holderMsp; suggestions.hidden = true;
  });
  render();
}

function voidForm(invoice) {
  const allowed = state.principal.role === 'ISSUER' && invoice.status !== 'VOIDED' && invoice.issuerMspId === state.principal.mspId;
  if (!allowed) return invoice.status === 'VOIDED' ? `<p class="detail-sub">已作废：${safe(invoice.voidReason || '未填写原因')}</p>` : '';
  return `<form id="void-form" class="transfer-form"><label>作废原因<input name="reason" required minlength="2" placeholder="例如：开票信息录入错误"></label><div><small>作废不会删除原记录，会新增一笔链上作废交易。</small></div><div></div><button class="danger" type="submit">确认作废</button></form>`;
}

function voidRequestForm(invoice, request) {
  if (invoice.status === 'VOIDED') return `<p class="detail-sub">已作废：${safe(invoice.voidReason || '未填写原因')}</p>`;
  const project = state.projects.find(item => item.id === invoice.projectId);
  const canRequest = hasRole('PROJECT_MEMBER') && project?.applicant === state.principal?.username;
  const canReview = hasRole('ISSUER') && invoice.issuerMspId === state.principal?.mspId && request?.status === 'PENDING_REVIEW';
  const requestInfo = request ? `<div class="void-request-summary"><b>当前申请：${safe(voidRequestStatusText(request.status))}</b><span>申请人：@${safe(request.applicant)} · ${safe(displayTime(request.updatedAt))}</span><p>申请原因：${safe(request.reason)}</p>${request.reviewOpinion ? `<p>审核意见：${safe(request.reviewOpinion)}${request.reviewer ? `（@${safe(request.reviewer)}）` : ''}</p>` : ''}</div>` : '';
  const reviewForm = canReview ? `<form id="void-review-form" class="review-form"><label>审核意见<textarea name="opinion" required minlength="2" maxlength="1000" placeholder="例如：确认开票信息有误，同意作废；或该票已用于报销，暂不允许作废。"></textarea></label><div><button type="submit" name="decision" value="APPROVE" class="danger">批准并正式作废</button><button type="submit" name="decision" value="REJECT" class="secondary">驳回申请</button></div></form>` : '';
  const requestForm = canRequest && (!request || request.status === 'REJECTED') ? `<form id="void-request-form" class="review-form"><label>申请作废原因<textarea name="reason" required minlength="2" maxlength="200" placeholder="例如：发票号码或金额录入错误，需要重新开具。"></textarea></label><div><button type="submit">提交作废申请</button></div></form>` : '';
  if (!requestInfo && !requestForm) return '<p class="detail-sub">仅该发票关联项目的申请人可提交作废申请；开票员可直接作废本组织开具的发票。</p>';
  return `${requestInfo}${reviewForm}${requestForm}`;
}

async function openDetail(id) {
  try {
    const [invoice, flows, history, voidRequest] = await Promise.all([api(`/invoices/${encodeURIComponent(id)}`), api(`/invoices/${encodeURIComponent(id)}/flows`), api(`/invoices/${encodeURIComponent(id)}/history`), api(`/invoices/${encodeURIComponent(id)}/void-request`)]);
    $('#detail-content').innerHTML = `<p class="eyebrow">ON-CHAIN INVOICE</p><h2 class="detail-title">${safe(invoice.invoiceNo)}</h2><p class="detail-sub">${safe(invoice.id)} · ${safe(statusText(invoice.status))}</p>
      <div class="details"><div><span>销售方</span><b>${safe(invoice.issuer)}</b></div><div><span>购买方</span><b>${safe(invoice.buyer)}</b></div><div><span>当前持有人</span><b>${safe(invoice.currentHolder)}</b></div><div><span>发行组织</span><b>${safe(invoice.issuerMspId || '旧版记录')}</b></div><div><span>持有人组织</span><b>${safe(invoice.holderMspId || '旧版记录')}</b></div><div><span>价税合计</span><b>${money(invoice.totalCents, invoice.currency)}</b></div></div>
      <div class="detail-section"><h3>内容指纹</h3><p class="hash">${safe(invoice.dataHash)}</p><button class="secondary" data-fill-verify="${safe(invoice.id)}" data-hash="${safe(invoice.dataHash)}">填入核验中心</button></div>
      <div class="detail-section"><h3>发票流转</h3><div class="timeline">${[...flows].sort((left, right) => Date.parse(left.timestamp) - Date.parse(right.timestamp)).map(flow => `<div><b>${flow.type === 'ISSUE' ? '开具存证' : flow.type === 'VOID' ? '发票作废' : flow.type === 'VOID_REQUEST' ? '提交作废申请' : flow.type === 'VOID_REJECTED' ? '作废申请已驳回' : '流转上链'}：${safe(flow.from)} → ${safe(flow.to)}</b><small>${safe(displayTime(flow.timestamp))} · 签名组织：${safe(flow.operator)}</small></div>`).join('') || '<p>暂无流转记录</p>'}</div></div>
      <div class="detail-section"><h3>执行流转</h3>${transferForm(invoice)}</div>
      <div class="detail-section"><h3>项目组作废申请</h3>${voidRequestForm(invoice, voidRequest)}</div>
      <div class="detail-section"><h3>作废 / 红冲</h3>${voidForm(invoice)}</div>
      <div class="detail-section"><h3>链上历史（${history.length} 笔）</h3><div class="timeline">${history.map(record => `<div><b>${safe(record.txId.slice(0, 22))}…</b><small>${safe(displayTime(record.timestamp))} · ${record.isDelete ? '删除' : `状态：${safe(record.value?.status || '')}`}</small></div>`).join('') || '<p>暂无历史记录</p>'}</div></div>`;
    $('#detail-dialog').showModal();
    const transfer = $('#transfer-form');
    if (transfer) { bindHolderPicker(transfer); transfer.addEventListener('submit', event => submitTransfer(event, invoice.id)); }
    $('#void-form')?.addEventListener('submit', event => submitVoid(event, invoice.id));
    $('#void-request-form')?.addEventListener('submit', event => submitVoidRequest(event, invoice.id));
    $('#void-review-form')?.addEventListener('submit', event => submitVoidReview(event, invoice.id));
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

async function submitVoidRequest(event, id) {
  event.preventDefault();
  const reason = new FormData(event.currentTarget).get('reason')?.trim();
  if (!reason) return;
  try { await api(`/invoices/${encodeURIComponent(id)}/void-request`, { method: 'POST', body: JSON.stringify({ reason }) }); $('#detail-dialog').close(); toast('作废申请已上链，等待开票员审核'); await loadInvoices(); }
  catch (error) { showAlert(error.message, '作废申请提交失败'); }
}

async function submitVoidReview(event, id) {
  event.preventDefault();
  const decision = event.submitter?.value; const opinion = new FormData(event.currentTarget).get('opinion')?.trim();
  if (!decision || !opinion) return;
  try { await api(`/invoices/${encodeURIComponent(id)}/void-request/review`, { method: 'POST', body: JSON.stringify({ decision, opinion }) }); $('#detail-dialog').close(); toast(decision === 'APPROVE' ? '已批准作废，发票状态已写入链上' : '已驳回作废申请，审核意见已写入链上'); await loadInvoices(); }
  catch (error) { showAlert(error.message, '作废申请审核失败'); }
}

async function openProjectDetail(id) {
  try {
    const project = state.projects.find(item => item.id === id);
    const events = await api(`/projects/${encodeURIComponent(id)}/events`);
    $('#detail-content').innerHTML = `<p class="eyebrow">ON-CHAIN PROJECT</p><h2 class="detail-title">${safe(project?.name || '项目详情')}</h2><p class="detail-sub">${safe(projectStatusText(project?.status))}</p>
      <div class="details"><div><span>申请人</span><b>${safe(project?.applicant)}</b></div><div><span>预算</span><b>${money(project?.budgetCents || 0)}</b></div><div><span>可用余额</span><b>${money(project?.availableCents || 0)}</b></div><div><span>冻结额度</span><b>${money(project?.reservedCents || 0)}</b></div><div><span>已支付</span><b>${money(project?.paidCents || 0)}</b></div><div><span>预期结项</span><b>${safe(project?.expectedEndDate)}</b></div></div>
      <div class="detail-section"><h3>项目内容</h3><p>${safe(project?.content)}</p></div><div class="detail-section"><h3>最近审核意见</h3><p>${safe(project?.reviewOpinion || '暂无')}</p></div>
      <div class="detail-section"><h3>链上项目事件</h3><div class="timeline">${events.map(item => `<div><b>${safe(item.type)} · ${safe(item.actor)}</b><small>${safe(displayTime(item.timestamp))}${item.note ? ` · ${safe(item.note)}` : ''}</small></div>`).join('') || '<p>暂无事件</p>'}</div></div>
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
  if (action === 'view' || action === 'review' || action === 'pay') { openReimbursementDetail(id); return; }
}

function reimbursementReviewForm(reimbursement) {
  if (reimbursement.status !== 'PENDING_REVIEW' || !hasRole('PROJECT_REVIEWER')) return '';
  return `<div class="detail-section review-section"><h3>报销审核</h3><p class="detail-sub">请核对项目、发票和金额，填写意见后再决定是否通过。</p><form id="reimbursement-review-form" class="review-form"><label>审核意见<textarea name="opinion" required minlength="2" maxlength="1000" placeholder="例如：发票与项目用途相符，同意报销；或请补充付款凭证。"></textarea></label><div><button type="submit" name="decision" value="APPROVE">审核通过并冻结额度</button><button type="submit" name="decision" value="REVISION" class="secondary">退回修改</button></div></form></div>`;
}

function reimbursementPaymentForm(reimbursement) {
  if (reimbursement.status !== 'APPROVED_RESERVED' || !hasRole('FINANCE_ADMIN')) return '';
  return `<div class="detail-section"><h3>支付确认</h3><div class="payment-confirmation"><p>确认实际款项已支付后，系统会将 <b>${money(reimbursement.amountCents)}</b> 从该项目的“冻结额度”转入“已支付金额”。此操作不可撤销。</p><button id="reimbursement-pay-form" type="button">确认已完成支付</button></div></div>`;
}

function openReimbursementDetail(id) {
  const reimbursement = state.reimbursements.find(item => item.id === id);
  if (!reimbursement) return;
  const project = state.projects.find(item => item.id === reimbursement.projectId);
  const invoice = state.invoices.find(item => item.id === reimbursement.invoiceId);
  $('#detail-content').innerHTML = `<p class="eyebrow">ON-CHAIN REIMBURSEMENT</p><h2 class="detail-title">报销单详情</h2><p class="detail-sub">${safe(reimbursementStatusText(reimbursement.status))} · 提交于 ${safe(displayTime(reimbursement.createdAt))}</p>
    <div class="details"><div><span>关联项目</span><b>${safe(project?.name || reimbursement.projectId)}</b></div><div><span>关联发票</span><b>${safe(invoice?.invoiceNo || reimbursement.invoiceId)}</b></div><div><span>报销金额</span><b>${money(reimbursement.amountCents)}</b></div><div><span>申请人</span><b>@${safe(reimbursement.applicant)}</b></div><div><span>审核人</span><b>${safe(reimbursement.reviewer ? `@${reimbursement.reviewer}` : '待审核')}</b></div><div><span>最后更新</span><b>${safe(displayTime(reimbursement.updatedAt))}</b></div></div>
    <div class="detail-section"><h3>审核意见</h3><p>${safe(reimbursement.reviewOpinion || '暂无审核意见')}</p></div>
    ${reimbursementReviewForm(reimbursement)}${reimbursementPaymentForm(reimbursement)}`;
  $('#detail-dialog').showModal();
  $('#reimbursement-review-form')?.addEventListener('submit', event => submitReimbursementReview(event, reimbursement.id));
  $('#reimbursement-pay-form')?.addEventListener('click', () => payReimbursement(reimbursement.id));
}

async function submitReimbursementReview(event, id) {
  event.preventDefault();
  const decision = event.submitter?.value; const opinion = new FormData(event.currentTarget).get('opinion')?.trim();
  if (!decision || !opinion) return;
  try {
    await api(`/reimbursements/${encodeURIComponent(id)}/review`, { method: 'POST', body: JSON.stringify({ decision, opinion }) });
    $('#detail-dialog').close(); toast(decision === 'APPROVE' ? '报销已审核通过，项目额度已冻结' : '报销单已退回修改'); await reloadBusinessData();
  } catch (error) { showAlert(error.message, '报销审核失败'); }
}

async function payReimbursement(id) {
  try {
    await api(`/reimbursements/${encodeURIComponent(id)}/pay`, { method: 'POST' });
    $('#detail-dialog').close(); toast('报销款已支付，资金池已更新'); await reloadBusinessData();
  } catch (error) { showAlert(error.message, '支付失败'); }
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

$('#organization-form').addEventListener('submit', async event => {
  event.preventDefault();
  const form = event.currentTarget; const payload = Object.fromEntries(new FormData(form));
  const result = $('#organization-result'); result.hidden = true;
  if (payload.type === 'PROJECT_TEAM' && !payload.parentId) {
    showAlert('内部项目组必须选择一个已登记的主体组织。', '组织登记失败'); return;
  }
  try {
    await api('/organizations', { method: 'POST', body: JSON.stringify(payload) });
    result.hidden = false; result.textContent = '组织登记成功，组织档案已写入链上目录。';
    form.reset(); updateOrganizationParentOptions(); await loadOrganizations(); toast('业务组织已登记并写入链上');
  } catch (error) { showAlert(error.message, '组织登记失败'); }
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

$('#logout').addEventListener('click', async () => { await api('/auth/logout', { method: 'POST' }); state.principal = null; state.invoices = []; state.organizations = []; state.users = []; state.projects = []; state.reimbursements = []; applyPrincipal(); $('#invoice-rows').innerHTML = '<tr><td colspan="6" class="empty">请登录后查看账本</td></tr>'; $('#project-rows').innerHTML = '<tr><td colspan="6" class="empty">请登录后查看项目</td></tr>'; $('#reimbursement-rows').innerHTML = '<tr><td colspan="6" class="empty">请登录后查看报销单</td></tr>'; $('#organization-rows').innerHTML = '<p class="empty">请登录后查看链上组织目录</p>'; showLoginView(); });
$('#search').addEventListener('input', renderInvoices);
$('#ocr-file').addEventListener('change', event => { if (event.currentTarget.files[0]) setOCRFile(event.currentTarget.files[0]); });
['dragenter', 'dragover'].forEach(type => $('#ocr-drop-zone').addEventListener(type, event => { event.preventDefault(); event.stopPropagation(); $('#ocr-drop-zone').classList.add('dragging'); }));
['dragleave', 'drop'].forEach(type => $('#ocr-drop-zone').addEventListener(type, event => { event.preventDefault(); event.stopPropagation(); $('#ocr-drop-zone').classList.remove('dragging'); }));
$('#ocr-drop-zone').addEventListener('drop', event => {
  const file = event.dataTransfer?.files?.[0];
  if (file) setOCRFile(file);
});
$('#ocr-recognize').addEventListener('click', recognizeInvoiceFile);
$('#refresh').addEventListener('click', loadInvoices);
$('#refresh-organizations').addEventListener('click', loadOrganizations);
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
$('#organization-type').addEventListener('change', updateOrganizationParentOptions);
$('#organization-rows').addEventListener('click', event => { const id = event.target.closest('[data-organization-detail]')?.dataset.organizationDetail; if (id) openOrganizationDetail(id); });
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
