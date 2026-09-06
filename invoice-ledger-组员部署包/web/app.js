const state = { invoices: [], projectInvoices: [], invoiceTotal: 0, invoicePage: 1, invoicePageSize: 20, invoiceQuery: '', invoiceUpdatedAt: '', dataStale: false, organizations: [], registrationOrganizations: [], principal: null, projects: [], reimbursements: [], users: [], editingProjectId: null, ocrFile: null, ocrSuggestion: null, ocrRequestToken: 0, sessionVersion: 0, pendingMutationKeys: new Set() };
const $ = (selector) => document.querySelector(selector);
let projectInvoiceRequest = 0;
let invoiceListRequest = 0;
let projectInvoiceLoadState = 'idle';
let projectInvoiceLoadError = '';
const activeReimbursements = () => state.reimbursements.filter(item => item.status !== 'WITHDRAWN');
const api = async (path, options = {}) => {
  const method = (options.method || 'GET').toUpperCase();
  const mutationKey = `${method}:${path}`;
  const isMutation = !['GET', 'HEAD'].includes(method);
  if (isMutation && state.pendingMutationKeys.has(mutationKey)) throw new Error('相同操作正在处理中，请勿重复提交。');
  if (isMutation) state.pendingMutationKeys.add(mutationKey);
  const defaultHeaders = options.body instanceof FormData ? {} : { 'Content-Type': 'application/json' };
  try {
    let response;
    try { response = await fetch(`/api${path}`, { ...options, credentials: 'same-origin', headers: { ...defaultHeaders, ...(options.headers || {}) } }); }
    catch (_) { throw new Error('无法连接应用服务，请检查后端是否启动以及网络连接。'); }
    if (response.status === 204) return null;
    let data;
    try { data = await response.json(); }
    catch (_) { throw new Error('服务返回的数据不完整或格式异常，请刷新后重试；若持续出现，请检查后端日志。'); }
    if (!response.ok) {
      if (response.status === 401 && path !== '/auth/me') showLogin();
      throw new Error(data.error || '请求失败，请检查 Fabric 网络和服务日志');
    }
    return data;
  } finally {
    if (isMutation) state.pendingMutationKeys.delete(mutationKey);
  }
};
const cents = (value) => Math.round(Number(value || 0) * 100);
const money = (value, currency = 'CNY') => {
  try { return new Intl.NumberFormat('zh-CN', { style: 'currency', currency: currency || 'CNY', minimumFractionDigits: 2 }).format(value / 100); }
  catch (_) { return new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY', minimumFractionDigits: 2 }).format(value / 100); }
};
const safe = (value = '') => String(value).replace(/[&<>'"]/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[char]));
const displayTime = (value) => {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'medium', hour12: false, timeZone: 'Asia/Shanghai' }).format(date);
};
const statusText = (status) => ({ ISSUED: '已开具', IN_CIRCULATION: '流转中', VOIDED: '已作废' }[status] || status);
const voidRequestStatusText = (status) => ({ PENDING_REVIEW: '待开票员审核', APPROVED: '已批准并作废', REJECTED: '已驳回' }[status] || status);
const invoiceHistoryActionText = (record) => {
  const status = record.value?.status;
  if (status === 'ISSUED') return '创建发票存证';
  if (status === 'IN_CIRCULATION') return '更新发票责任人';
  if (status === 'VOIDED') return '发票作废';
  return record.isDelete ? '删除发票记录' : '更新发票记录';
};
const projectStatusText = (status) => ({ DRAFT: '草稿', PENDING_REVIEW: '待立项审核', REVISION_REQUIRED: '需修改', EXECUTING: '执行中（未拨款）', CLOSURE_REVIEW: '待结项审核', FINANCIAL_SETTLEMENT: '验收通过，财务结算中', CLOSURE_APPROVED: '验收通过（旧记录）', ARCHIVED: '已结算归档' }[status] || status);
const reimbursementStatusText = (status) => ({ PENDING_REVIEW: '待报销审核', REVISION_REQUIRED: '材料需修改', APPROVED_RESERVED: '已审核，额度已冻结', PAID: '已支付', WITHDRAWN: '已撤回' }[status] || status);
const roleText = (role) => ({ ISSUER: '开票员', HOLDER: '跨组织流转员', AUDITOR: '审计员', PROJECT_MEMBER: '项目组成员', PROJECT_REVIEWER: '项目管理审核员', FINANCE_ADMIN: '财务管理员', ORG_ADMIN: '组织管理员' }[role] || role);
const organizationText = (mspId) => ({ Org1MSP: 'Org1 · Fabric 节点 1', Org2MSP: 'Org2 · Fabric 节点 2' }[mspId] || mspId);
const organizationTypeText = (type) => ({ PRIMARY: '牵头单位（学校 / 总部）', PROJECT_TEAM: '公司内部项目组', EXTERNAL: '外部协作组织' }[type] || type);
const hasRole = (...roles) => Boolean(state.principal && roles.includes(state.principal.role));

function switchView(view, updateHash = true) {
  const target = document.getElementById(view);
  if (!target || !target.classList.contains('app-view')) return;
  document.querySelectorAll('.app-view').forEach(item => item.classList.toggle('active', item === target));
  document.querySelectorAll('[data-view]').forEach(item => item.classList.toggle('active', item.dataset.view === view));
  if (updateHash) history.replaceState(null, '', `#${view}`);
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

// Register navigation before optional page widgets. This keeps the sidebar
// usable even when a browser temporarily combines a cached older HTML page
// with a newer JavaScript asset during development.
document.addEventListener('click', event => {
  const button = event.target.closest('[data-view]');
  if (!button) return;
  switchView(button.dataset.view);
});

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

function resetOCRState(clearForm = false) {
  state.ocrRequestToken += 1; state.ocrFile = null; state.ocrSuggestion = null;
  $('#ocr-result').hidden = true; $('#ocr-file-name').textContent = '尚未选择文件'; $('#ocr-recognize').disabled = true;
  $('#ocr-file').value = '';
  if (!clearForm) return;
  const form = $('#invoice-form');
  ['id', 'invoiceNo', 'issuer', 'buyer', 'amount', 'tax'].forEach(name => { form.elements[name].value = ''; });
  form.elements.issueDate.value = new Date().toISOString().slice(0, 10);
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
  const token = state.ocrRequestToken;
  const button = $('#ocr-recognize'); const resultNode = $('#ocr-result');
  button.disabled = true; button.textContent = '识别中…'; resultNode.hidden = true;
  try {
    const payload = new FormData(); payload.append('file', file, file.name);
    const result = await api('/ocr/invoice', { method: 'POST', body: payload });
    if (token !== state.ocrRequestToken || file !== state.ocrFile) return;
    applyOCRFields(result.fields); renderOCRResult(result); toast('OCR 已完成基础回填，请人工确认后上链');
  } catch (error) { showAlert(error.message, '发票 OCR 识别失败'); }
  finally { if (token === state.ocrRequestToken) { button.disabled = !state.ocrFile; button.textContent = '开始识别'; } }
}

function setOCRFile(file) {
  if (!file) return;
  resetOCRState(true);
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

function showDetailError(message, title = '操作未完成') {
  const node = $('#detail-error');
  if (!node) { showAlert(message, title); return; }
  node.hidden = false;
  node.innerHTML = `<b>${safe(title)}</b><span>${safe(message)}</span>`;
  node.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

function setFormBusy(form, busy) {
  if (!form) return;
  form.querySelectorAll('button[type="submit"], button[data-submit-action]').forEach(button => { button.disabled = busy; });
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
  updateOrganizationMSPOptions();
  $('#identity-banner').hidden = !principal;
  if (principal) {
    $('#identity-name').textContent = principal.displayName;
    $('#identity-role').textContent = roleText(principal.role);
    const organization = state.organizations.find(item => item.id === principal.organizationId) || state.registrationOrganizations.find(item => item.id === principal.organizationId);
    $('#identity-msp').textContent = organization ? `${organization.name} · ${principal.mspId}` : principal.mspId;
  }
  if (!principal) renderStatistics();
}

function updateOrganizationMSPOptions() {
  const select = $('#organization-msp');
  const note = $('#organization-msp-note');
  if (!select || !note) return;
  const mspId = state.principal?.mspId;
  if (!mspId) {
    select.innerHTML = '<option value="">请先登录组织管理员</option>';
    select.disabled = true;
    note.textContent = '';
    return;
  }
  select.disabled = false;
  select.innerHTML = `<option value="${safe(mspId)}">${safe(organizationText(mspId))}</option>`;
  note.textContent = `组织档案将由当前登录管理员所在的 ${organizationText(mspId)} 签名登记。若要在另一 Fabric 节点登记，请退出后使用该节点的组织管理员账号登录。`;
}

function renderHolderOptions() {
  const registeredHolders = state.users.filter(user => user.status === 'ACTIVE' && user.role === 'HOLDER');
  const demoHolder = { username: 'holder-org2', displayName: 'Org2 跨组织流转员（演示）', mspId: 'Org2MSP', role: 'HOLDER', status: 'ACTIVE' };
  const holders = registeredHolders.some(user => user.username === demoHolder.username) ? registeredHolders : [...registeredHolders, demoHolder];
  $('#holder-options').innerHTML = holders.map(user => `<option value="${safe(user.username)}">${safe(user.displayName)} · ${safe(user.mspId)}</option>`).join('');
}

async function loadUsers() {
  if (!state.principal) return;
  const sessionVersion = state.sessionVersion;
  try { const users = await api('/users'); if (sessionVersion !== state.sessionVersion || !state.principal) return; state.users = users; renderHolderOptions(); renderOrganizations(); }
  catch (error) { state.dataStale = true; renderStatistics(); showAlert(error.message, '链上用户读取失败'); }
}

function updateOrganizationParentOptions() {
  const isProjectTeam = $('#organization-type').value === 'PROJECT_TEAM';
  const wrap = $('#organization-parent-wrap'); const select = $('#organization-parent');
  wrap.hidden = !isProjectTeam;
  if (!isProjectTeam) { select.value = ''; return; }
  const primaryOrganizations = state.organizations.filter(item => item.type === 'PRIMARY' && item.status === 'ACTIVE' && (!state.principal || item.mspId === state.principal.mspId));
  const selected = select.value;
  select.innerHTML = primaryOrganizations.length
    ? `<option value="">请选择牵头单位</option>${primaryOrganizations.map(item => `<option value="${safe(item.id)}">${safe(item.name)}</option>`).join('')}`
    : '<option value="">请先登记牵头单位</option>';
  select.disabled = primaryOrganizations.length === 0;
  if (primaryOrganizations.some(item => item.id === selected)) select.value = selected;
}

function organizationMemberScope(organization) {
  const ids = new Set([organization.id]);
  const pending = [organization.id];
  while (pending.length) {
    const parentID = pending.pop();
    state.organizations.filter(item => item.parentId === parentID).forEach(child => {
      if (!ids.has(child.id)) { ids.add(child.id); pending.push(child.id); }
    });
  }
  return ids;
}

function organizationMembers(organization) {
  const scope = organizationMemberScope(organization);
  return state.users
    .filter(user => scope.has(user.organizationId) && user.status === 'ACTIVE')
    .sort((left, right) => left.displayName.localeCompare(right.displayName, 'zh-CN'));
}

function renderOrganizations() {
  const organizations = [...state.organizations].sort((left, right) => left.type.localeCompare(right.type) || left.name.localeCompare(right.name, 'zh-CN'));
  $('#organization-rows').innerHTML = organizations.length ? organizations.map(item => {
    const parent = state.organizations.find(candidate => candidate.id === item.parentId);
    const members = organizationMembers(item);
    const directCount = state.users.filter(user => user.organizationId === item.id && user.status === 'ACTIVE').length;
    const hasSuborganizations = organizationMemberScope(item).size > 1;
    const memberLabel = hasSuborganizations ? `查看成员（${members.length}，含下属 ${members.length - directCount}）` : `查看成员（${members.length}）`;
    return `<article class="organization-card" data-organization-detail="${safe(item.id)}"><div class="organization-card-heading"><span class="organization-type ${safe(item.type.toLowerCase())}">${safe(organizationTypeText(item.type))}</span><span class="member-status">${item.status === 'ACTIVE' ? '在用' : safe(item.status)}</span></div><h3>${safe(item.name)}</h3><p>${safe(item.description || '暂无组织说明')}</p><dl><div><dt>所属牵头单位</dt><dd>${safe(parent?.name || '—')}</dd></div><div><dt>链上接入节点</dt><dd>${safe(organizationText(item.mspId))}</dd></div><div><dt>登记人</dt><dd>@${safe(item.createdBy)}</dd></div></dl><button class="secondary organization-members-button" type="button" data-organization-detail="${safe(item.id)}">${safe(memberLabel)}</button></article>`;
  }).join('') : '<p class="empty">暂无链上业务组织。请先由组织管理员登记牵头单位。</p>';
  updateOrganizationParentOptions();
}

async function loadOrganizations() {
  if (!state.principal) return;
  const sessionVersion = state.sessionVersion;
  try { const organizations = await api('/organizations'); if (sessionVersion !== state.sessionVersion || !state.principal) return; state.organizations = organizations; renderOrganizations(); }
  catch (error) { state.dataStale = true; renderStatistics(); $('#organization-rows').innerHTML = `<p class="empty">${safe(error.message)}</p>`; showAlert(error.message, '组织目录读取失败'); }
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
  const scope = organizationMemberScope(organization);
  const members = organizationMembers(organization);
  const hasSuborganizations = scope.size > 1;
  $('#detail-content').innerHTML = `<p class="eyebrow">ON-CHAIN BUSINESS ORGANIZATION</p><h2 class="detail-title">${safe(organization.name)}</h2><p class="detail-sub">${safe(organizationTypeText(organization.type))} · ${safe(organization.status === 'ACTIVE' ? '在用' : organization.status)}</p>
    <div class="details"><div><span>所属牵头单位</span><b>${safe(parent?.name || '—')}</b></div><div><span>账本接入节点</span><b>${safe(organizationText(organization.mspId))}</b></div><div><span>登记人</span><b>@${safe(organization.createdBy)}</b></div><div><span>登记时间</span><b>${safe(displayTime(organization.createdAt))}</b></div></div>
    <div class="detail-section"><h3>组织说明</h3><p>${safe(organization.description || '暂无组织说明')}</p></div>
    <div class="detail-section"><h3>组织成员（${members.length} 位${hasSuborganizations ? '，含下属项目组' : ''}）</h3>${members.length ? `<div class="organization-member-list">${members.map(user => { const userOrganization = state.organizations.find(item => item.id === user.organizationId); return `<div><span class="member-avatar">${safe(user.displayName.slice(0, 1))}</span><p><b>${safe(user.displayName)}</b><small>@${safe(user.username)} · ${safe(roleText(user.role))}${hasSuborganizations ? ` · ${safe(userOrganization?.name || '直属成员')}` : ''}</small></p><em>${safe(organizationText(user.mspId))}</em></div>`; }).join('')}</div>` : '<p class="detail-sub">该组织及其下属项目组暂未绑定业务用户。请在注册页选择对应组织后注册成员。</p>'}</div>`;
  $('#detail-dialog').showModal();
}

function renderProjectOptions() {
  const available = state.projects.filter(project => ['FINANCIAL_SETTLEMENT', 'CLOSURE_APPROVED'].includes(project.status));
  const invoiceProject = $('#invoice-project'); const reimbursementProject = $('#reimbursement-project');
  const invoiceSelected = invoiceProject.value; const reimbursementSelected = reimbursementProject.value;
  const invoiceProjects = state.projects.filter(project => ['EXECUTING', 'FINANCIAL_SETTLEMENT', 'CLOSURE_APPROVED'].includes(project.status));
  const options = available.map(project => `<option value="${safe(project.id)}">${safe(project.name)}（可用 ${money(project.availableCents)}）</option>`).join('');
  invoiceProject.innerHTML = `<option value="">不关联项目</option>${invoiceProjects.map(project => `<option value="${safe(project.id)}">${safe(project.name)}</option>`).join('')}`;
  reimbursementProject.innerHTML = `<option value="">请选择财务结算中的项目</option>${options}`;
  if (invoiceProjects.some(project => project.id === invoiceSelected)) invoiceProject.value = invoiceSelected;
  if (available.some(project => project.id === reimbursementSelected)) reimbursementProject.value = reimbursementSelected;
  updateProjectDetailButtons();
  if (reimbursementProject.value) loadProjectInvoices(reimbursementProject.value); else renderReimbursementInvoiceOptions();
}

function selectedProject(selectId) { return state.projects.find(project => project.id === $(selectId).value); }

function updateProjectDetailButtons() {
  const invoiceProject = selectedProject('#invoice-project'); const reimbursementProject = selectedProject('#reimbursement-project');
  $('#invoice-project-detail').disabled = !invoiceProject;
  $('#reimbursement-project-detail').disabled = !reimbursementProject;
}

function renderReimbursementInvoiceOptions() {
  const select = $('#reimbursement-invoice'); const projectID = $('#reimbursement-project').value;
  const hint = $('#reimbursement-invoice-hint'); const list = $('#reimbursement-invoice-list');
  if (projectID && ['loading', 'error'].includes(projectInvoiceLoadState)) {
    select.innerHTML = '<option value="">' + (projectInvoiceLoadState === 'loading' ? '正在加载发票…' : '发票加载失败，请重新选择项目') + '</option>';
    select.disabled = true;
    $('#reimbursement-invoice-detail').disabled = true;
    $('#clear-reimbursement-selection').disabled = false;
    if (hint) hint.textContent = projectInvoiceLoadState === 'loading' ? '正在读取项目发票，请稍候。' : projectInvoiceLoadError;
    if (list) list.innerHTML = '';
    return;
  }
  const selected = select.value; const usedInvoiceIDs = new Set(activeReimbursements().map(item => item.invoiceId));
  const projectInvoices = state.projectInvoices.filter(invoice => invoice.projectId === projectID);
  const activeInvoices = projectInvoices.filter(invoice => invoice.status !== 'VOIDED');
  const invoices = activeInvoices.filter(invoice => !usedInvoiceIDs.has(invoice.id));
  select.innerHTML = projectID ? `<option value="">请选择该项目下的发票</option>${invoices.map(invoice => `<option value="${safe(invoice.id)}">${safe(invoice.invoiceNo)} · ${safe(invoice.issuer)} · ${money(invoice.totalCents, invoice.currency)}</option>`).join('')}` : '<option value="">请先选择项目</option>';
  select.disabled = !projectID || invoices.length === 0;
  if (invoices.some(invoice => invoice.id === selected)) select.value = selected;
  const invoiceDetail = $('#reimbursement-invoice-detail');
  if (invoiceDetail) invoiceDetail.disabled = !select.value;
  const clearSelection = $('#clear-reimbursement-selection');
  if (clearSelection) clearSelection.disabled = !projectID && !select.value;
  const hintText = !projectID ? '请先选择处于财务结算阶段的项目。'
    : projectInvoices.length === 0 ? '该项目尚未关联发票。请由项目成员在“发票存证”时选择该项目，或在发票详情中执行“补关联项目”。'
      : activeInvoices.length === 0 ? '该项目关联的发票均已作废，不能用于报销。'
        : invoices.length === 0 ? '该项目的有效发票均已关联报销单；一张发票不能重复报销。'
          : `已加载 ${invoices.length} 张可报销发票；请选择一张后可查看详情。`;
  if (hint) hint.textContent = hintText;
  if (!projectID) {
    if (list) list.innerHTML = '';
    return;
  }
  if (!list) return;
  list.innerHTML = projectInvoices.length ? `<p class="invoice-list-title">项目关联发票</p>${projectInvoices.map(invoice => {
    const voided = invoice.status === 'VOIDED'; const used = usedInvoiceIDs.has(invoice.id);
    const selectable = !voided && !used;
    const status = voided ? '已作废，不能报销' : used ? '已关联其他报销单' : '可用于本次报销';
    return `<article class="reimbursement-invoice-item ${selectable ? 'available' : 'unavailable'}"><div><b>${safe(invoice.invoiceNo)}</b><small>${safe(invoice.issuer)} · ${money(invoice.totalCents, invoice.currency)}</small><em>${safe(status)}</em></div><div class="reimbursement-invoice-actions">${selectable ? `<button type="button" class="secondary compact" data-reimbursement-invoice-select="${safe(invoice.id)}">选用</button>` : ''}<button type="button" class="secondary compact" data-reimbursement-invoice-view="${safe(invoice.id)}">详情</button></div></article>`;
  }).join('')}` : '<p class="detail-sub">该项目目前没有关联发票。</p>';
}

async function loadProjectInvoices(projectID) {
  const request = ++projectInvoiceRequest;
  const session = state.sessionVersion;
  state.projectInvoices = [];
  projectInvoiceLoadState = projectID ? 'loading' : 'idle';
  projectInvoiceLoadError = '';
  renderReimbursementInvoiceOptions();
  if (!projectID) return;
  const current = () => request === projectInvoiceRequest && session === state.sessionVersion && !!state.principal && $('#reimbursement-project').value === projectID;
  try {
    const invoices = await api(`/projects/${encodeURIComponent(projectID)}/invoices`);
    if (!current()) return;
    state.projectInvoices = invoices;
    projectInvoiceLoadState = 'ready';
  } catch (error) {
    if (!current()) return;
    projectInvoiceLoadState = 'error';
    projectInvoiceLoadError = error.message;
  }
  renderReimbursementInvoiceOptions();
}

function projectActions(project) {
  const buttons = [`<button class="link-button" data-project-action="view" data-project-id="${safe(project.id)}">详情</button>`];
  const ownProject = project.applicant === state.principal?.username;
  if (ownProject && ['DRAFT', 'REVISION_REQUIRED'].includes(project.status) && hasRole('ISSUER', 'PROJECT_MEMBER')) {
    buttons.push(`<button class="link-button" data-project-action="edit" data-project-id="${safe(project.id)}">编辑</button><button class="link-button" data-project-action="submit" data-project-id="${safe(project.id)}">提交</button>`);
  }
  if (ownProject && project.status === 'EXECUTING' && hasRole('ISSUER', 'PROJECT_MEMBER')) buttons.push(`<button class="link-button" data-project-action="closure" data-project-id="${safe(project.id)}">申请结项</button>`);
  if (project.status === 'FINANCIAL_SETTLEMENT' && hasRole('FINANCE_ADMIN')) buttons.push(`<button class="link-button" data-project-action="finalize" data-project-id="${safe(project.id)}">完成结算并归档</button>`);
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
  const sessionVersion = state.sessionVersion;
  try { const projects = await api('/projects'); if (sessionVersion !== state.sessionVersion || !state.principal) return; state.projects = projects; renderProjects(); renderReimbursements(); renderStatistics(); }
  catch (error) { state.dataStale = true; renderStatistics(); $('#project-rows').innerHTML = `<tr><td colspan="6" class="empty">${safe(error.message)}</td></tr>`; showAlert(error.message, '项目读取失败'); }
}

function reimbursementActions(reimbursement) {
  const buttons = [`<button class="link-button" data-reimbursement-action="view" data-reimbursement-id="${safe(reimbursement.id)}">详情</button>`];
  if (reimbursement.status === 'PENDING_REVIEW' && hasRole('PROJECT_REVIEWER')) buttons.push(`<button class="link-button" data-reimbursement-action="review" data-reimbursement-id="${safe(reimbursement.id)}">审核</button>`);
  if (reimbursement.status === 'REVISION_REQUIRED' && reimbursement.applicant === state.principal?.username) buttons.push(`<button class="link-button" data-reimbursement-action="resubmit" data-reimbursement-id="${safe(reimbursement.id)}">修改并重提</button>`);
  if (['PENDING_REVIEW', 'REVISION_REQUIRED', 'APPROVED_RESERVED'].includes(reimbursement.status) && reimbursement.applicant === state.principal?.username) buttons.push(`<button class="link-button" data-reimbursement-action="withdraw" data-reimbursement-id="${safe(reimbursement.id)}">撤回</button>`);
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
  const sessionVersion = state.sessionVersion;
  try { const reimbursements = await api('/reimbursements'); if (sessionVersion !== state.sessionVersion || !state.principal) return; state.reimbursements = reimbursements; renderReimbursements(); renderReimbursementInvoiceOptions(); renderStatistics(); }
  catch (error) { state.dataStale = true; renderStatistics(); $('#reimbursement-rows').innerHTML = `<tr><td colspan="6" class="empty">${safe(error.message)}</td></tr>`; showAlert(error.message, '报销单读取失败'); }
}

async function reloadBusinessData() { state.dataStale = false; await Promise.all([loadUsers(), loadOrganizations(), loadInvoices(), loadProjects(), loadReimbursements()]); renderStatistics(); }

const shanghaiDateKey = (value) => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return new Date(date.getTime() + 8 * 60 * 60 * 1000).toISOString().slice(0, 10);
};

const shanghaiMonthKey = (value) => shanghaiDateKey(value).slice(0, 7);

function recentDateKeys(days) {
  const today = new Date(Date.now() + 8 * 60 * 60 * 1000);
  const keys = [];
  for (let index = days - 1; index >= 0; index -= 1) {
    const date = new Date(today); date.setUTCDate(today.getUTCDate() - index);
    keys.push(date.toISOString().slice(0, 10));
  }
  return keys;
}

function recentMonthKeys(months) {
  const today = new Date(Date.now() + 8 * 60 * 60 * 1000);
  const keys = [];
  for (let index = months - 1; index >= 0; index -= 1) {
    const date = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth() - index, 1));
    keys.push(date.toISOString().slice(0, 7));
  }
  return keys;
}

function lineChart(labels, values) {
  const width = 640; const height = 236; const left = 38; const right = 20; const top = 18; const bottom = 38;
  const maxValue = Math.max(...values, 0); const scaleMax = Math.max(maxValue, 1); const chartWidth = width - left - right; const chartHeight = height - top - bottom;
  const point = (value, index) => `${(left + chartWidth * index / Math.max(values.length - 1, 1)).toFixed(1)},${(top + chartHeight - (value / scaleMax) * chartHeight).toFixed(1)}`;
  const points = values.map(point).join(' ');
  const grid = [0, 0.5, 1].map(ratio => { const y = top + chartHeight - chartHeight * ratio; return `<line x1="${left}" y1="${y}" x2="${width - right}" y2="${y}" class="chart-grid"/>`; }).join('');
  const dots = values.map((value, index) => { const [x, y] = point(value, index).split(','); return `<circle cx="${x}" cy="${y}" r="4" class="chart-dot"><title>${safe(labels[index])}：${safe(money(value))}</title></circle>`; }).join('');
  const axisLabels = labels.map((label, index) => `<text x="${left + (chartWidth * index / Math.max(labels.length - 1, 1))}" y="${height - 12}" text-anchor="middle" class="chart-label">${safe(label.slice(5).replace('-', '/'))}</text>`).join('');
  return `<svg class="chart-svg" viewBox="0 0 ${width} ${height}" role="img" aria-label="近七天报销申请金额折线图">${grid}<text x="${left}" y="13" class="chart-value-label">最高 ${safe(money(maxValue))}</text><polyline points="${points}" class="chart-line"/>${dots}${axisLabels}</svg>`;
}

function barChart(labels, values) {
  const width = 640; const height = 236; const left = 28; const right = 16; const top = 18; const bottom = 40;
  const maxValue = Math.max(...values, 0); const scaleMax = Math.max(maxValue, 1); const chartWidth = width - left - right; const chartHeight = height - top - bottom; const slot = chartWidth / values.length; const barWidth = Math.min(54, slot * 0.58);
  const bars = values.map((value, index) => { const barHeight = value / scaleMax * chartHeight; const x = left + slot * index + (slot - barWidth) / 2; const y = top + chartHeight - barHeight; return `<rect x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${barWidth.toFixed(1)}" height="${barHeight.toFixed(1)}" rx="4" class="chart-bar"><title>${safe(labels[index])}：${safe(money(value))}</title></rect><text x="${(x + barWidth / 2).toFixed(1)}" y="${height - 13}" text-anchor="middle" class="chart-label">${safe(labels[index].slice(2).replace('-', '/'))}</text>`; }).join('');
  return `<svg class="chart-svg" viewBox="0 0 ${width} ${height}" role="img" aria-label="近六个月实际支付金额柱状图"><line x1="${left}" y1="${top + chartHeight}" x2="${width - right}" y2="${top + chartHeight}" class="chart-grid"/><text x="${left}" y="13" class="chart-value-label">最高 ${safe(money(maxValue))}</text>${bars}</svg>`;
}

function donutChart(items) {
  const total = items.reduce((sum, item) => sum + item.value, 0);
  if (total === 0) return '<p class="chart-empty">暂无报销单数据</p>';
  const radius = 58; const circumference = 2 * Math.PI * radius; let offset = 0;
  const arcs = items.filter(item => item.value > 0).map(item => { const length = item.value / total * circumference; const arc = `<circle cx="88" cy="88" r="${radius}" fill="none" stroke="${item.color}" stroke-width="22" stroke-dasharray="${length} ${circumference - length}" stroke-dashoffset="${-offset}" transform="rotate(-90 88 88)"><title>${safe(item.label)}：${item.value} 笔</title></circle>`; offset += length; return arc; }).join('');
  const legend = items.map(item => `<li><i style="background:${item.color}"></i><span>${safe(item.label)}</span><b>${item.value} 笔</b></li>`).join('');
  return `<div class="donut-layout"><svg class="donut-svg" viewBox="0 0 176 176" role="img" aria-label="报销单状态分布环形图"><circle cx="88" cy="88" r="${radius}" fill="none" stroke="#edf1f4" stroke-width="22"/>${arcs}<text x="88" y="83" text-anchor="middle" class="donut-total">${total}</text><text x="88" y="103" text-anchor="middle" class="chart-label">报销单</text></svg><ul class="chart-legend">${legend}</ul></div>`;
}

function renderStatistics() {
  const summary = $('#statistics-summary');
  if (!summary) return;
  if (!state.principal) {
    summary.querySelectorAll('strong').forEach(item => { item.textContent = '—'; });
    ['#reimbursement-daily-chart', '#payment-monthly-chart', '#reimbursement-status-chart'].forEach(selector => { $(selector).innerHTML = '<p class="chart-empty">登录后读取统计数据</p>'; });
    return;
  }
  $('#statistics-stale').hidden = !state.dataStale;
  const totals = state.projects.reduce((result, project) => ({ budget: result.budget + project.budgetCents, approved: result.approved + (['FINANCIAL_SETTLEMENT', 'ARCHIVED', 'CLOSURE_APPROVED'].includes(project.status) ? project.budgetCents : 0), reserved: result.reserved + project.reservedCents, paid: result.paid + project.paidCents }), { budget: 0, approved: 0, reserved: 0, paid: 0 });
  [totals.budget, totals.approved, totals.reserved, totals.paid].forEach((value, index) => { summary.children[index].querySelector('strong').textContent = money(value); });
  const dailyKeys = recentDateKeys(7); const dailyValues = dailyKeys.map(key => state.reimbursements.filter(item => shanghaiDateKey(item.createdAt) === key).reduce((sum, item) => sum + item.amountCents, 0));
  const monthKeys = recentMonthKeys(6); const monthlyValues = monthKeys.map(key => state.reimbursements.filter(item => item.status === 'PAID' && shanghaiMonthKey(item.updatedAt) === key).reduce((sum, item) => sum + item.amountCents, 0));
  const statusItems = [
    { label: '待审核', value: state.reimbursements.filter(item => item.status === 'PENDING_REVIEW').length, color: '#e3ae46' },
    { label: '需修改', value: state.reimbursements.filter(item => item.status === 'REVISION_REQUIRED').length, color: '#d66a6a' },
    { label: '已冻结', value: state.reimbursements.filter(item => item.status === 'APPROVED_RESERVED').length, color: '#639fd6' },
    { label: '已支付', value: state.reimbursements.filter(item => item.status === 'PAID').length, color: '#62a57a' },
  ];
  $('#reimbursement-daily-chart').innerHTML = lineChart(dailyKeys, dailyValues);
  $('#payment-monthly-chart').innerHTML = barChart(monthKeys, monthlyValues);
  $('#reimbursement-status-chart').innerHTML = donutChart(statusItems);
}

function updateMetrics() {
  const summary = state.invoiceSummary;
  $('#metric-total').textContent = summary ? summary.total : '—';
  $('#metric-circulating').textContent = summary ? summary.circulating : '—';
  $('#metric-amount').textContent = summary ? money(summary.amountCents) : '—';
}

function renderInvoices() {
  const list = state.invoices;
  $('#invoice-count').textContent = `共 ${state.invoiceTotal} 条记录${state.invoiceUpdatedAt ? ` · 更新于 ${displayTime(state.invoiceUpdatedAt)}` : ''}`;
  $('#invoice-rows').innerHTML = list.length ? list.map(item => `<tr>
    <td><strong>${safe(item.invoiceNo)}</strong><small>${safe(item.id)} · ${safe(item.issueDate)}</small></td>
    <td><strong>${safe(item.issuer)}</strong><small>→ ${safe(item.buyer)}</small></td>
    <td>${money(item.totalCents, item.currency)}</td><td>${safe(item.currentHolder)}<small>${safe(item.holderMspId || '旧版记录')}</small></td>
    <td><span class="status ${item.status === 'IN_CIRCULATION' ? 'circulating' : item.status === 'VOIDED' ? 'voided' : ''}">${statusText(item.status)}</span></td>
    <td><button class="link-button" data-detail="${safe(item.id)}">详情</button></td></tr>`).join('') : '<tr><td colspan="6" class="empty">没有符合条件的发票记录</td></tr>';
}

async function loadInvoices() {
  if (!state.principal) return;
  const sessionVersion = state.sessionVersion;
  const request = ++invoiceListRequest;
  const current = () => request === invoiceListRequest && sessionVersion === state.sessionVersion && Boolean(state.principal);
  try { const data = await api(`/invoices?page=${state.invoicePage}&pageSize=${state.invoicePageSize}&q=${encodeURIComponent(state.invoiceQuery)}`); if (!current()) return; state.invoices = data.items || []; state.invoiceSummary = data.summary || null; state.invoiceTotal = data.total || 0; state.invoicePage = data.page || 1; state.invoiceUpdatedAt = data.updatedAt || ''; updateMetrics(); renderInvoices(); renderReimbursementInvoiceOptions(); renderReimbursements(); renderInvoicePagination(); }
  catch (error) { if (!current()) return; state.invoiceSummary = null; updateMetrics(); state.dataStale = true; renderStatistics(); $('#invoice-count').textContent = '数据未更新'; $('#invoice-rows').innerHTML = `<tr><td colspan="6" class="empty">${safe(error.message)}</td></tr>`; showAlert(error.message, '账本读取失败'); }
}

function renderInvoicePagination() {
  const node = $('#invoice-pagination'); if (!node) return;
  const pages = Math.max(1, Math.ceil(state.invoiceTotal / state.invoicePageSize));
  node.hidden = state.invoiceTotal <= state.invoicePageSize;
  node.innerHTML = `<button id="invoice-prev" class="secondary compact" type="button" ${state.invoicePage <= 1 ? 'disabled' : ''}>上一页</button><span>第 ${state.invoicePage} / ${pages} 页</span><button id="invoice-next" class="secondary compact" type="button" ${state.invoicePage >= pages ? 'disabled' : ''}>下一页</button>`;
  $('#invoice-prev')?.addEventListener('click', () => { state.invoicePage -= 1; loadInvoices(); });
  $('#invoice-next')?.addEventListener('click', () => { state.invoicePage += 1; loadInvoices(); });
}

function transferForm(invoice, transfer) {
  if (transfer?.status === 'PENDING') {
    if (transfer.to === state.principal?.username) return `<p class="detail-sub">@${safe(transfer.from)} 请求将责任交接给你。${transfer.note ? ` 交接说明：${safe(transfer.note)}` : ''}</p><form id="transfer-response-form" class="review-form"><label>回应说明（可选）<textarea name="response" maxlength="500" placeholder="例如：已收到纸质票据"></textarea></label><div><button type="submit" name="decision" value="ACCEPT">确认接收</button><button type="submit" name="decision" value="REJECT" class="secondary">拒绝接收</button></div></form>`;
    if (transfer.from === state.principal?.username) return `<p class="detail-sub">交接申请已发给 @${safe(transfer.to)}，等待对方确认。${transfer.note ? ` 说明：${safe(transfer.note)}` : ''}</p><button id="cancel-transfer" class="secondary" type="button">撤回交接申请</button>`;
    return `<p class="detail-sub">当前有待确认的交接：@${safe(transfer.from)} → @${safe(transfer.to)}。</p>`;
  }
  const allowed = hasRole('ISSUER', 'HOLDER', 'PROJECT_MEMBER') && invoice.status !== 'VOIDED' && invoice.currentHolder === state.principal?.username && invoice.holderMspId === state.principal.mspId;
  if (!allowed) return '<p class="detail-sub">该发票当前无需你处理。只有当前责任人可在确有材料交接需要时发起交接。</p>';
  return `<form id="transfer-form" class="transfer-form"><label>接收人<input name="to" autocomplete="off" required placeholder="输入账号或姓名搜索成员"></label><label>接收组织<select name="toMspId"><option value="Org1MSP">Org1MSP</option><option value="Org2MSP">Org2MSP</option></select></label><label>交接说明（可选）<input name="note" maxlength="500" placeholder="例如：已交接纸质票据原件"></label><div><small>提交后责任不会立刻变化，需由接收人确认。</small></div><button type="submit">发起交接申请</button><div id="holder-suggestions" class="holder-suggestions" hidden></div></form>`;
}

function bindHolderPicker(form) {
  const input = form.elements.to; const mspSelect = form.elements.toMspId; const suggestions = $('#holder-suggestions');
  const registeredHolders = state.users.filter(user => user.status === 'ACTIVE' && ['HOLDER', 'PROJECT_MEMBER', 'ISSUER'].includes(user.role));
  const demoHolder = { username: 'holder-org2', displayName: 'Org2 跨组织流转员（演示）', mspId: 'Org2MSP', role: 'HOLDER', status: 'ACTIVE' };
  const holders = registeredHolders.some(user => user.username === demoHolder.username) ? registeredHolders : [...registeredHolders, demoHolder];
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
  const canRequest = hasRole('PROJECT_MEMBER', 'ISSUER') && Boolean(invoice.projectId);
  const canReview = hasRole('ISSUER') && invoice.issuerMspId === state.principal?.mspId && request?.status === 'PENDING_REVIEW';
  const requestInfo = request ? `<div class="void-request-summary"><b>当前申请：${safe(voidRequestStatusText(request.status))}</b><span>申请人：@${safe(request.applicant)} · ${safe(displayTime(request.updatedAt))}</span><p>申请原因：${safe(request.reason)}</p>${request.reviewOpinion ? `<p>审核意见：${safe(request.reviewOpinion)}${request.reviewer ? `（@${safe(request.reviewer)}）` : ''}</p>` : ''}</div>` : '';
  const reviewForm = canReview ? `<form id="void-review-form" class="review-form"><label>审核意见<textarea name="opinion" required minlength="2" maxlength="1000" placeholder="例如：确认开票信息有误，同意作废；或该票已用于报销，暂不允许作废。"></textarea></label><div><button type="submit" name="decision" value="APPROVE" class="danger">批准并正式作废</button><button type="submit" name="decision" value="REJECT" class="secondary">驳回申请</button></div></form>` : '';
  const requestForm = canRequest && (!request || request.status === 'REJECTED') ? `<form id="void-request-form" class="review-form"><label>申请作废原因<textarea name="reason" required minlength="2" maxlength="200" placeholder="例如：发票号码或金额录入错误，需要重新开具。"></textarea></label><div><button type="submit">提交作废申请</button></div></form>` : '';
  if (!requestInfo && !requestForm) return '<p class="detail-sub">仅关联项目成员可提交作废申请；开票员可直接作废本组织开具的发票。</p>';
  return `${requestInfo}${reviewForm}${requestForm}`;
}

async function openDetail(id) {
  try {
    const invoice = await api(`/invoices/${encodeURIComponent(id)}`);
    const [flowResult, historyResult, voidRequestResult, transferResult] = await Promise.allSettled([api(`/invoices/${encodeURIComponent(id)}/flows`), api(`/invoices/${encodeURIComponent(id)}/history`), api(`/invoices/${encodeURIComponent(id)}/void-request`), api(`/invoices/${encodeURIComponent(id)}/transfer`)]);
    const flows = flowResult.status === 'fulfilled' ? flowResult.value : [];
    const history = historyResult.status === 'fulfilled' ? historyResult.value : [];
    const voidRequest = voidRequestResult.status === 'fulfilled' ? voidRequestResult.value : null;
    const transferRequest = transferResult.status === 'fulfilled' ? transferResult.value : null;
    const partialLoadWarning = [flowResult, historyResult, voidRequestResult, transferResult].some(item => item.status === 'rejected') ? '<p class="detail-sub">部分追溯记录暂时未加载；发票主信息仍可正常查看和操作。</p>' : '';
    $('#detail-content').innerHTML = `<div id="detail-error" class="detail-error" hidden></div><p class="eyebrow">ON-CHAIN INVOICE</p><h2 class="detail-title">${safe(invoice.invoiceNo)}</h2><p class="detail-sub">${safe(invoice.id)} · ${safe(statusText(invoice.status))}</p>${partialLoadWarning}
      <div class="details"><div><span>销售方</span><b>${safe(invoice.issuer)}</b></div><div><span>购买方</span><b>${safe(invoice.buyer)}</b></div><div><span>当前持有人</span><b>${safe(invoice.currentHolder)}</b></div><div><span>发行组织</span><b>${safe(invoice.issuerMspId || '旧版记录')}</b></div><div><span>持有人组织</span><b>${safe(invoice.holderMspId || '旧版记录')}</b></div><div><span>价税合计</span><b>${money(invoice.totalCents, invoice.currency)}</b></div></div>
      <div class="detail-section"><h3>内容核验</h3><p class="detail-sub">核验时请根据待核验的票面填写关键内容，系统会在服务端重新计算摘要，不会将链上摘要自动填入。</p><button class="secondary" data-fill-verify="${safe(invoice.id)}">前往核验中心</button></div>
      <div class="detail-section"><h3>发票流转</h3><div class="timeline">${[...flows].sort((left, right) => Date.parse(left.timestamp) - Date.parse(right.timestamp)).map(flow => `<div><b>${({ ISSUE: '开具存证', CORRECTION: '创建更正版本', VOID: '发票作废', VOID_REQUEST: '提交作废申请', VOID_REJECTED: '作废申请已驳回', TRANSFER_REQUEST: '发起材料交接', TRANSFER_ACCEPTED: '确认材料交接', TRANSFER_REJECTED: '拒绝材料交接', TRANSFER_CANCELLED: '撤回材料交接', TRANSFER: '旧版直接流转' }[flow.type] || flow.type)}：${safe(flow.from)} → ${safe(flow.to)}</b><small>${safe(displayTime(flow.timestamp))} · 签名组织：${safe(flow.operator)}</small></div>`).join('') || '<p>暂无流转记录</p>'}</div></div>
      <div class="detail-section"><h3>材料交接</h3>${transferForm(invoice, transferRequest)}</div>
      <div class="detail-section">${projectAssociationForm(invoice)}</div>
      ${invoice.status === 'VOIDED' ? `<div class="detail-section">${correctionForm(invoice)}</div>` : ''}
      <div class="detail-section"><h3>项目组作废申请</h3>${voidRequestForm(invoice, voidRequest)}</div>
      <div class="detail-section"><h3>作废 / 红冲</h3>${voidForm(invoice)}</div>
      <div class="detail-section"><div class="detail-title-row"><h3>链上操作记录（${history.length} 笔）</h3><button id="toggle-transaction-ids" class="secondary compact" type="button">查看交易编码</button></div><p class="detail-sub">记录发票主数据在 Fabric 中发生的状态变化。</p><div class="timeline">${[...history].sort((left, right) => Date.parse(left.timestamp) - Date.parse(right.timestamp)).map(record => `<div><b>${safe(invoiceHistoryActionText(record))}</b><small>${safe(displayTime(record.timestamp))} · 当前状态：${safe(statusText(record.value?.status || '已删除'))}</small><small class="transaction-id" hidden>交易编码：${safe(record.txId)}</small></div>`).join('') || '<p>暂无历史记录</p>'}</div></div>`;
    $('#detail-dialog').showModal();
    const transfer = $('#transfer-form');
    if (transfer) { bindHolderPicker(transfer); transfer.addEventListener('submit', event => submitTransfer(event, invoice.id)); }
    $('#transfer-response-form')?.addEventListener('submit', event => respondTransfer(event, invoice.id));
    $('#cancel-transfer')?.addEventListener('click', () => cancelTransfer(invoice.id));
    $('#link-project-form')?.addEventListener('submit', event => linkInvoiceProject(event, invoice.id));
    $('#unlink-project')?.addEventListener('click', () => unlinkInvoiceProject(invoice.id));
    $('#correction-form')?.addEventListener('submit', event => submitCorrection(event, invoice.id));
    $('#void-form')?.addEventListener('submit', event => submitVoid(event, invoice.id));
    $('#void-request-form')?.addEventListener('submit', event => submitVoidRequest(event, invoice.id));
    $('#void-review-form')?.addEventListener('submit', event => submitVoidReview(event, invoice.id));
    $('#toggle-transaction-ids')?.addEventListener('click', event => {
      const visible = event.currentTarget.dataset.visible === 'true';
      document.querySelectorAll('.transaction-id').forEach(item => { item.hidden = visible; });
      event.currentTarget.dataset.visible = String(!visible);
      event.currentTarget.textContent = visible ? '查看交易编码' : '隐藏交易编码';
    });
    document.querySelector('[data-fill-verify]').addEventListener('click', event => { $('#verify-id').value = event.currentTarget.dataset.fillVerify; $('#detail-dialog').close(); switchView('verify'); });
  } catch (error) { showAlert(error.message, '发票详情读取失败'); }
}

async function submitTransfer(event, id) {
  event.preventDefault(); const form = event.currentTarget;
  setFormBusy(form, true);
  try { await api(`/invoices/${encodeURIComponent(id)}/transfers`, { method: 'POST', body: JSON.stringify(Object.fromEntries(new FormData(form))) }); toast('交接申请已提交，等待接收方确认'); await openDetail(id); await loadInvoices(); }
  catch (error) { showDetailError(error.message, '流转失败'); }
  finally { setFormBusy(form, false); }
}

async function respondTransfer(event, id) {
  event.preventDefault(); const form = event.currentTarget; const decision = event.submitter?.value; if (!decision) return;
  setFormBusy(form, true);
  try { await api(`/invoices/${encodeURIComponent(id)}/transfers/respond`, { method: 'POST', body: JSON.stringify({ decision, response: new FormData(form).get('response')?.trim() || '' }) }); toast(decision === 'ACCEPT' ? '已确认接收，责任人已更新' : '已拒绝该交接申请'); await openDetail(id); await loadInvoices(); }
  catch (error) { showDetailError(error.message, '交接回应失败'); }
  finally { setFormBusy(form, false); }
}

async function cancelTransfer(id) {
  try { await api(`/invoices/${encodeURIComponent(id)}/transfers/cancel`, { method: 'POST' }); toast('交接申请已撤回'); await openDetail(id); }
  catch (error) { showDetailError(error.message, '撤回交接失败'); }
}

function linkProjectForm(invoice) {
  const projects = state.projects.filter(project => ['EXECUTING', 'FINANCIAL_SETTLEMENT', 'CLOSURE_APPROVED'].includes(project.status));
  if (!hasRole('PROJECT_MEMBER', 'ISSUER') || !projects.length) return '<h3>补关联项目</h3><p class="detail-sub">该发票尚未关联项目。登录对应项目的成员账号后，可在这里补关联一个可用项目。</p>';
  return `<h3>补关联项目</h3><p class="detail-sub">仅未报销、未作废且尚未关联项目的发票可补关联；不会修改发票票面内容。</p><form id="link-project-form" class="review-form"><label>选择项目<select name="projectId" required><option value="">请选择项目</option>${projects.map(project => `<option value="${safe(project.id)}">${safe(project.name)}</option>`).join('')}</select></label><div><button type="submit">确认关联项目</button></div></form>`;
}

function projectAssociationForm(invoice) {
  if (!invoice.projectId) return linkProjectForm(invoice);
  const project = state.projects.find(item => item.id === invoice.projectId);
  const canUnlink = hasRole('PROJECT_MEMBER', 'ISSUER') && invoice.status !== 'VOIDED' && project?.status !== 'ARCHIVED';
  const used = activeReimbursements().some(item => item.invoiceId === invoice.id);
  return `<h3>关联项目</h3><p class="detail-sub">当前关联：${safe(project?.name || invoice.projectId)}。${used ? '该发票已关联报销单，不能取消项目关联。' : '取消后可重新关联其他符合条件的项目；关联与取消记录都会保留在链上项目事件中。'}</p>${canUnlink && !used ? '<button id="unlink-project" class="secondary" type="button">取消项目关联</button>' : ''}`;
}

function correctionForm(invoice) {
  const allowed = hasRole('PROJECT_MEMBER', 'ISSUER') && invoice.issuerMspId === state.principal?.mspId;
  if (!allowed) return '<h3>创建更正版本</h3><p class="detail-sub">仅原开具组织的开票员或项目成员可基于已作废记录创建更正版本。</p>';
  return `<h3>创建更正版本</h3><p class="detail-sub">原作废存证会永久保留。更正版本必须使用相同发票号码，但可修正录入错误的日期、主体和金额。</p><form id="correction-form" class="form-grid"><label>发票号码<input name="invoiceNo" required value="${safe(invoice.invoiceNo)}"></label><label>开票日期<input name="issueDate" type="date" required value="${safe(invoice.issueDate)}"></label><label>销售方<input name="issuer" required value="${safe(invoice.issuer)}"></label><label>购买方<input name="buyer" required value="${safe(invoice.buyer)}"></label><label>购买方关联组织<select name="buyerMspId"><option value="Org1MSP" ${invoice.buyerMspId === 'Org1MSP' ? 'selected' : ''}>Org1MSP</option><option value="Org2MSP" ${invoice.buyerMspId === 'Org2MSP' ? 'selected' : ''}>Org2MSP</option></select></label><label>金额（元）<input name="amount" required type="number" min="0" step="0.01" value="${(invoice.amountCents / 100).toFixed(2)}"></label><label>税额（元）<input name="tax" required type="number" min="0" step="0.01" value="${(invoice.taxCents / 100).toFixed(2)}"></label><input name="currency" type="hidden" value="CNY"><input name="projectId" type="hidden" value="${safe(invoice.projectId || '')}"><div class="form-action"><button type="submit">创建更正版本</button></div></form>`;
}

async function linkInvoiceProject(event, id) {
  event.preventDefault(); const form = event.currentTarget; setFormBusy(form, true);
  try { await api(`/invoices/${encodeURIComponent(id)}/project`, { method: 'POST', body: JSON.stringify(Object.fromEntries(new FormData(form))) }); toast('发票已补关联项目'); await reloadBusinessData(); await openDetail(id); }
  catch (error) { showDetailError(error.message, '关联项目失败'); }
  finally { setFormBusy(form, false); }
}

async function unlinkInvoiceProject(id) {
  if (!window.confirm('确认取消这张发票与当前项目的关联吗？该操作会写入链上项目事件。')) return;
  try { await api(`/invoices/${encodeURIComponent(id)}/project`, { method: 'DELETE' }); toast('已取消项目关联，可按需要重新关联其他项目'); await reloadBusinessData(); await openDetail(id); }
  catch (error) { showDetailError(error.message, '取消项目关联失败'); }
}

async function submitCorrection(event, id) {
  event.preventDefault(); const form = event.currentTarget; const payload = Object.fromEntries(new FormData(form)); payload.amountCents = cents(payload.amount); payload.taxCents = cents(payload.tax); delete payload.amount; delete payload.tax;
  setFormBusy(form, true);
  try { const result = await api(`/invoices/${encodeURIComponent(id)}/corrections`, { method: 'POST', body: JSON.stringify(payload) }); toast(`已创建更正版本：${result.id}`); $('#detail-dialog').close(); await reloadBusinessData(); }
  catch (error) { showDetailError(error.message, '创建更正版本失败'); }
  finally { setFormBusy(form, false); }
}

async function submitVoid(event, id) {
  event.preventDefault(); const form = event.currentTarget;
  setFormBusy(form, true);
  try { await api(`/invoices/${encodeURIComponent(id)}/void`, { method: 'POST', body: JSON.stringify(Object.fromEntries(new FormData(form))) }); toast('发票已作废，原始记录和历史均已保留'); $('#detail-dialog').close(); await loadInvoices(); }
  catch (error) { showDetailError(error.message, '作废失败'); }
  finally { setFormBusy(form, false); }
}

async function submitVoidRequest(event, id) {
  event.preventDefault(); const form = event.currentTarget;
  const reason = new FormData(form).get('reason')?.trim();
  if (!reason) return;
  setFormBusy(form, true);
  try { await api(`/invoices/${encodeURIComponent(id)}/void-request`, { method: 'POST', body: JSON.stringify({ reason }) }); $('#detail-dialog').close(); toast('作废申请已上链，等待开票员审核'); await loadInvoices(); }
  catch (error) { showDetailError(error.message, '作废申请提交失败'); }
  finally { setFormBusy(form, false); }
}

async function submitVoidReview(event, id) {
  event.preventDefault(); const form = event.currentTarget;
  const decision = event.submitter?.value; const opinion = new FormData(form).get('opinion')?.trim();
  if (!decision || !opinion) return;
  setFormBusy(form, true);
  try { await api(`/invoices/${encodeURIComponent(id)}/void-request/review`, { method: 'POST', body: JSON.stringify({ decision, opinion }) }); $('#detail-dialog').close(); toast(decision === 'APPROVE' ? '已批准作废，发票状态已写入链上' : '已驳回作废申请，审核意见已写入链上'); await loadInvoices(); }
  catch (error) { showDetailError(error.message, '作废申请审核失败'); }
  finally { setFormBusy(form, false); }
}

async function openProjectDetail(id) {
  try {
    const project = state.projects.find(item => item.id === id);
    const [events, members] = await Promise.all([api(`/projects/${encodeURIComponent(id)}/events`), api(`/projects/${encodeURIComponent(id)}/members`)]);
    const eventText = (type) => ({ CREATE_DRAFT: '创建项目草稿', UPDATE_DRAFT: '更新项目草稿', SUBMIT_APPLICATION: '提交立项申请', PROJECT_APPROVE: '立项审核通过（待验收后拨款）', PROJECT_REVISION: '立项退回修改', REQUEST_CLOSURE: '提交结项申请', CLOSURE_APPROVE_AND_RELEASE_FUND: '验收通过并发放结算额度', CLOSURE_REVISION: '结项退回修改', FINALIZE_SETTLEMENT: '完成财务结算并归档', ADD_MEMBER: '添加项目成员', LINK_INVOICE: '补关联项目发票', SUBMIT_REIMBURSEMENT: '提交报销申请', REIMBURSEMENT_APPROVE: '报销审核通过并冻结额度', REIMBURSEMENT_REVISION: '报销退回修改', RESUBMIT_REIMBURSEMENT: '重新提交报销申请', WITHDRAW_REIMBURSEMENT: '撤回报销申请', PAY_REIMBURSEMENT: '确认报销支付' }[type] || type);
    const canManageMembers = project?.applicant === state.principal?.username && hasRole('PROJECT_MEMBER', 'ISSUER') && project.status !== 'ARCHIVED';
    $('#detail-content').innerHTML = `<div id="detail-error" class="detail-error" hidden></div><p class="eyebrow">ON-CHAIN PROJECT</p><h2 class="detail-title">${safe(project?.name || '项目详情')}</h2><p class="detail-sub">${safe(projectStatusText(project?.status))}</p>
      <div class="details"><div><span>项目负责人</span><b>${safe(project?.applicant)}</b></div><div><span>结项预算</span><b>${money(project?.budgetCents || 0)}</b></div><div><span>可用结算额度</span><b>${money(project?.availableCents || 0)}</b></div><div><span>冻结额度</span><b>${money(project?.reservedCents || 0)}</b></div><div><span>已支付</span><b>${money(project?.paidCents || 0)}</b></div><div><span>已回收</span><b>${money(project?.recoveredCents || 0)}</b></div><div><span>预期结项</span><b>${safe(project?.expectedEndDate)}</b></div></div>
      <div class="detail-section"><h3>项目内容</h3><p>${safe(project?.content)}</p></div>${project?.closureMaterials ? `<div class="detail-section"><h3>结项材料说明</h3><p>${safe(project.closureMaterials)}</p></div>` : ''}<div class="detail-section"><h3>最近审核意见</h3><p>${safe(project?.reviewOpinion || '暂无')}</p></div>
      <div class="detail-section"><h3>项目成员（${members.length} 位）</h3><div class="organization-member-list">${members.map(member => `<div><span class="member-avatar">${safe(member.username.slice(0, 1))}</span><p><b>@${safe(member.username)}</b><small>${member.role === 'LEADER' ? '项目负责人' : '项目成员'}</small></p></div>`).join('') || '<p>旧项目尚未建立成员名单；负责人保留原有权限。</p>'}</div>${canManageMembers ? `<form id="project-member-form" class="review-form"><label>添加本组织成员账号<input name="username" required placeholder="输入已注册的项目成员账号"></label><div><button type="submit">添加成员</button></div></form>` : ''}</div>
      <div class="detail-section"><h3>链上项目事件</h3><div class="timeline">${[...events].sort((left, right) => Date.parse(left.timestamp) - Date.parse(right.timestamp)).map(item => `<div><b>${safe(eventText(item.type))} · @${safe(item.actor)}</b><small>${safe(displayTime(item.timestamp))}${item.note ? ` · ${safe(item.note)}` : ''}${item.referenceId ? ` · 关联报销单：${safe(item.referenceId)}` : ''}</small></div>`).join('') || '<p>暂无事件</p>'}</div></div>
      ${projectReviewForm(project)}`;
    $('#detail-dialog').showModal();
    $('#project-review-form')?.addEventListener('submit', event => submitProjectReview(event, project.id));
    $('#project-member-form')?.addEventListener('submit', event => submitProjectMember(event, project.id));
  } catch (error) { showAlert(error.message, '项目详情读取失败'); }
}

async function submitProjectMember(event, id) {
  event.preventDefault(); const form = event.currentTarget; const username = new FormData(form).get('username')?.trim(); if (!username) return;
  setFormBusy(form, true);
  try { await api(`/projects/${encodeURIComponent(id)}/members`, { method: 'POST', body: JSON.stringify({ username }) }); toast('项目成员已添加'); await openProjectDetail(id); }
  catch (error) { showDetailError(error.message, '添加项目成员失败'); }
  finally { setFormBusy(form, false); }
}

function projectReviewForm(project) {
  if (!project || !hasRole('PROJECT_REVIEWER') || !['PENDING_REVIEW', 'CLOSURE_REVIEW'].includes(project.status)) return '';
  const closure = project.status === 'CLOSURE_REVIEW';
  return `<div class="detail-section review-section"><h3>${closure ? '结项审核' : '立项审核'}</h3><p class="detail-sub">请填写审核意见，再选择通过或要求项目组修改。</p><form id="project-review-form" class="review-form" data-endpoint="${closure ? 'closure-review' : 'review'}"><label>审核意见<textarea name="opinion" required minlength="2" maxlength="1000" placeholder="例如：项目目标清晰，同意立项；或请补充预算依据。"></textarea></label><div><button type="submit" name="decision" value="APPROVE">${closure ? '通过结项' : '通过立项'}</button><button type="submit" name="decision" value="REVISION" class="secondary">要求修改</button></div></form></div>`;
}

async function submitProjectReview(event, id) {
  event.preventDefault(); const form = event.currentTarget;
  const decision = event.submitter?.value;
  const opinion = new FormData(form).get('opinion')?.trim();
  if (!decision || !opinion) return;
  const endpoint = form.dataset.endpoint;
  setFormBusy(form, true);
  try {
    await api(`/projects/${encodeURIComponent(id)}/${endpoint}`, { method: 'POST', body: JSON.stringify({ decision, opinion }) });
    $('#detail-dialog').close(); toast(decision === 'APPROVE' ? '审核通过，结果已写入链上' : '已要求项目组修改，意见已写入链上'); await reloadBusinessData();
  } catch (error) { showDetailError(error.message, '项目审核失败'); }
  finally { setFormBusy(form, false); }
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
    if (action === 'finalize') {
      if (!window.confirm('确认完成财务结算并归档？系统将回收当前可用余额，归档后不能再新增发票或报销单。')) return;
      await api(`/projects/${encodeURIComponent(id)}/finalize`, { method: 'POST' });
    }
    if (action === 'review' || action === 'closure-review') { await openProjectDetail(id); return; }
    toast(action === 'submit' ? '项目申请已提交审核' : action === 'finalize' ? '项目已结算归档，剩余额度已回收' : '结项申请已提交审核'); await reloadBusinessData();
  } catch (error) { showAlert(error.message, '项目操作失败'); }
}

async function handleReimbursementAction(event) {
  const button = event.target.closest('[data-reimbursement-action]');
  if (!button) return;
  const { reimbursementAction: action, reimbursementId: id } = button.dataset;
  if (action === 'withdraw') { withdrawReimbursement(id); return; }
  if (action === 'view' || action === 'review' || action === 'pay' || action === 'resubmit') { openReimbursementDetail(id); return; }
}

function reimbursementReviewForm(reimbursement) {
  if (reimbursement.status !== 'PENDING_REVIEW' || !hasRole('PROJECT_REVIEWER')) return '';
  return `<div class="detail-section review-section"><h3>报销审核</h3><p class="detail-sub">请核对项目、发票和金额，填写意见后再决定是否通过。</p><form id="reimbursement-review-form" class="review-form"><label>审核意见<textarea name="opinion" required minlength="2" maxlength="1000" placeholder="例如：发票与项目用途相符，同意报销；或请补充付款凭证。"></textarea></label><div><button type="submit" name="decision" value="APPROVE">审核通过并冻结额度</button><button type="submit" name="decision" value="REVISION" class="secondary">退回修改</button></div></form></div>`;
}

function reimbursementPaymentForm(reimbursement) {
  if (reimbursement.status !== 'APPROVED_RESERVED' || !hasRole('FINANCE_ADMIN')) return '';
  return `<div class="detail-section"><h3>支付确认</h3><div class="payment-confirmation"><p>确认实际款项已支付后，系统会将 <b>${money(reimbursement.amountCents)}</b> 从该项目的“冻结额度”转入“已支付金额”。此操作不可撤销。</p><button id="reimbursement-pay-form" type="button">确认已完成支付</button></div></div>`;
}

function reimbursementResubmitForm(reimbursement) {
  if (reimbursement.status !== 'REVISION_REQUIRED' || reimbursement.applicant !== state.principal?.username) return '';
  return `<div class="detail-section review-section"><h3>补充材料并重新提交</h3><p class="detail-sub">请依据审核意见修改材料说明；会保留原提交记录，并将本次版本重新送审。</p><form id="reimbursement-resubmit-form" class="review-form"><label>材料说明<textarea name="evidence" required minlength="2" maxlength="4000">${safe(reimbursement.evidence || '')}</textarea></label><div><button type="submit">重新提交审核</button></div></form></div>`;
}

function reimbursementWithdrawForm(reimbursement) {
  if (!['PENDING_REVIEW', 'REVISION_REQUIRED', 'APPROVED_RESERVED'].includes(reimbursement.status) || reimbursement.applicant !== state.principal?.username) return '';
  const note = reimbursement.status === 'APPROVED_RESERVED' ? '该操作会释放已冻结的项目额度。' : '该操作会解除该发票与报销单的占用关系。';
  return `<div class="detail-section"><h3>撤回报销单</h3><p class="detail-sub">${note} 已支付报销单不能撤回。</p><button id="reimbursement-withdraw" class="danger" type="button">确认撤回</button></div>`;
}

async function openReimbursementDetail(id) {
  const reimbursement = state.reimbursements.find(item => item.id === id);
  if (!reimbursement) return;
  const project = state.projects.find(item => item.id === reimbursement.projectId);
  const session = state.sessionVersion;
  let invoice;
  try { invoice = await api(`/invoices/${encodeURIComponent(reimbursement.invoiceId)}`); if (!invoice?.id) throw new Error('关联发票数据不完整，暂不能审核或支付。'); }
  catch (error) { if (session === state.sessionVersion && state.principal) showAlert(error.message, '关联发票读取失败'); return; }
  if (session !== state.sessionVersion || !state.principal) return;
  $('#detail-content').innerHTML = `<div id="detail-error" class="detail-error" hidden></div><p class="eyebrow">ON-CHAIN REIMBURSEMENT</p><h2 class="detail-title">报销单详情</h2><p class="detail-sub">${safe(reimbursementStatusText(reimbursement.status))} · 提交于 ${safe(displayTime(reimbursement.createdAt))}</p>
    <div class="details"><div><span>关联项目</span><b>${safe(project?.name || reimbursement.projectId)}</b></div><div><span>关联发票</span><b>${safe(invoice?.invoiceNo || reimbursement.invoiceId)}</b></div><div><span>报销金额</span><b>${money(reimbursement.amountCents)}</b></div><div><span>申请人</span><b>@${safe(reimbursement.applicant)}</b></div><div><span>审核人</span><b>${safe(reimbursement.reviewer ? `@${reimbursement.reviewer}` : '待审核')}</b></div><div><span>最后更新</span><b>${safe(displayTime(reimbursement.updatedAt))}</b></div></div>
    <div class="detail-section"><h3>关联发票票面信息</h3><div class="details"><div><span>发票号码</span><b>${safe(invoice.invoiceNo)}</b></div><div><span>开票日期</span><b>${safe(invoice.issueDate)}</b></div><div><span>销售方</span><b>${safe(invoice.issuer)}</b></div><div><span>购买方</span><b>${safe(invoice.buyer)}</b></div><div><span>不含税金额</span><b>${money(invoice.amountCents, invoice.currency)}</b></div><div><span>税额</span><b>${money(invoice.taxCents, invoice.currency)}</b></div><div><span>价税合计</span><b>${money(invoice.totalCents, invoice.currency)}</b></div><div><span>发票状态</span><b>${safe(statusText(invoice.status))}</b></div></div></div>
    <div class="detail-section"><h3>材料说明</h3><p>${safe(reimbursement.evidence || '旧版记录仅保留材料摘要')}</p></div><div class="detail-section"><h3>审核意见</h3><p>${safe(reimbursement.reviewOpinion || '暂无审核意见')}</p></div>
    ${reimbursementReviewForm(reimbursement)}${reimbursementResubmitForm(reimbursement)}${reimbursementPaymentForm(reimbursement)}${reimbursementWithdrawForm(reimbursement)}`;
  $('#detail-dialog').showModal();
  $('#reimbursement-review-form')?.addEventListener('submit', event => submitReimbursementReview(event, reimbursement.id));
  $('#reimbursement-pay-form')?.addEventListener('click', () => payReimbursement(reimbursement.id));
  $('#reimbursement-resubmit-form')?.addEventListener('submit', event => resubmitReimbursement(event, reimbursement.id));
  $('#reimbursement-withdraw')?.addEventListener('click', () => withdrawReimbursement(reimbursement.id));
}

async function resubmitReimbursement(event, id) {
  event.preventDefault(); const form = event.currentTarget; const evidence = new FormData(form).get('evidence')?.trim(); if (!evidence) return;
  setFormBusy(form, true);
  try { await api(`/reimbursements/${encodeURIComponent(id)}/resubmit`, { method: 'POST', body: JSON.stringify({ evidence }) }); $('#detail-dialog').close(); toast('报销单已重新提交审核'); await reloadBusinessData(); }
  catch (error) { showDetailError(error.message, '报销单重新提交失败'); }
  finally { setFormBusy(form, false); }
}

async function withdrawReimbursement(id) {
  if (!window.confirm('确认撤回该报销单吗？未支付的冻结额度会被释放。')) return;
  try { await api(`/reimbursements/${encodeURIComponent(id)}/withdraw`, { method: 'POST' }); $('#detail-dialog').close(); toast('报销单已撤回，相关额度已同步处理'); await reloadBusinessData(); }
  catch (error) { showDetailError(error.message, '报销单撤回失败'); }
}

async function submitReimbursementReview(event, id) {
  event.preventDefault(); const form = event.currentTarget;
  const decision = event.submitter?.value; const opinion = new FormData(form).get('opinion')?.trim();
  if (!decision || !opinion) return;
  setFormBusy(form, true);
  try {
    await api(`/reimbursements/${encodeURIComponent(id)}/review`, { method: 'POST', body: JSON.stringify({ decision, opinion }) });
    $('#detail-dialog').close(); toast(decision === 'APPROVE' ? '报销已审核通过，项目额度已冻结' : '报销单已退回修改'); await reloadBusinessData();
  } catch (error) { showDetailError(error.message, '报销审核失败'); }
  finally { setFormBusy(form, false); }
}

async function payReimbursement(id) {
  try {
    await api(`/reimbursements/${encodeURIComponent(id)}/pay`, { method: 'POST' });
    $('#detail-dialog').close(); toast('报销款已支付，资金池已更新'); await reloadBusinessData();
  } catch (error) { showDetailError(error.message, '支付失败'); }
}

$('#project-form')?.addEventListener('submit', async event => {
  event.preventDefault();
  const form = event.currentTarget; const payload = Object.fromEntries(new FormData(form));
  payload.budgetCents = cents(payload.budget); delete payload.budget;
  $('#project-result').hidden = true;
  setFormBusy(form, true);
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
  finally { setFormBusy(form, false); }
});

$('#reimbursement-form')?.addEventListener('submit', async event => {
  event.preventDefault();
  const form = event.currentTarget; const payload = Object.fromEntries(new FormData(form));
  $('#reimbursement-result').hidden = true;
  setFormBusy(form, true);
  try {
    await api('/reimbursements', { method: 'POST', body: JSON.stringify(payload) });
    const node = $('#reimbursement-result'); node.hidden = false; node.textContent = '报销单已提交审核，请等待项目管理审核员处理。';
    form.reset(); updateProjectDetailButtons(); await loadProjectInvoices(''); toast('报销单已提交项目管理审核'); await loadReimbursements();
  } catch (error) { showAlert(error.message, '报销提交失败'); }
  finally { setFormBusy(form, false); }
});

$('#organization-form')?.addEventListener('submit', async event => {
  event.preventDefault();
  const form = event.currentTarget; const payload = Object.fromEntries(new FormData(form));
  const result = $('#organization-result'); result.hidden = true;
  if (payload.type === 'PROJECT_TEAM' && !payload.parentId) {
    showAlert('内部项目组必须选择一个已登记的牵头单位。', '组织登记失败'); return;
  }
  setFormBusy(form, true);
  try {
    await api('/organizations', { method: 'POST', body: JSON.stringify(payload) });
    result.hidden = false; result.textContent = '组织登记成功，组织档案已写入链上目录。';
    form.reset(); updateOrganizationParentOptions(); await loadOrganizations(); toast('业务组织已登记并写入链上');
  } catch (error) { showAlert(error.message, '组织登记失败'); }
  finally { setFormBusy(form, false); }
});

$('#invoice-form')?.addEventListener('submit', async event => {
  event.preventDefault(); const formElement = event.currentTarget; const form = new FormData(formElement); const payload = Object.fromEntries(form); payload.amountCents = cents(payload.amount); payload.taxCents = cents(payload.tax); delete payload.amount; delete payload.tax;
  $('#create-result').hidden = true;
  setFormBusy(formElement, true);
  try { await api('/invoices', { method: 'POST', body: JSON.stringify(payload) }); const node = $('#create-result'); node.hidden = false; node.textContent = '存证成功，发票已写入可信账本。'; toast('发票已完成链上存证'); formElement.reset(); $('#invoice-form [name="issueDate"]').value = new Date().toISOString().slice(0, 10); resetOCRState(false); await Promise.all([loadInvoices(), loadReimbursements()]); }
  catch (error) { showAlert(error.message, '存证失败'); }
  finally { setFormBusy(formElement, false); }
});

$('#verify-form')?.addEventListener('submit', async event => {
  event.preventDefault(); const form = event.currentTarget; const id = $('#verify-id').value.trim(); const payload = Object.fromEntries(new FormData(form)); payload.amountCents = cents(payload.amount); payload.taxCents = cents(payload.tax); delete payload.amount; delete payload.tax; const node = $('#verify-result');
  setFormBusy(form, true);
  try { const result = await api(`/invoices/${encodeURIComponent(id)}/verify`, { method: 'POST', body: JSON.stringify(payload) }); node.className = `verify-result ${result.valid ? 'success' : 'failure'}`; node.innerHTML = result.valid ? `✓ 核验通过：发票 <strong>${safe(result.invoice.invoiceNo)}</strong> 的票面关键内容与链上存证一致，且当前有效。` : result.dataHashMatched ? `! 票面内容与链上原始存证一致，但该发票当前状态为<strong>${safe(statusText(result.invoice.status))}</strong>，不能继续用于报销。` : `× 核验未通过：${safe(result.reason)}。请逐项检查票面内容。`; }
  catch (error) { node.className = 'verify-result failure'; node.textContent = error.message; showAlert(error.message, '核验失败'); }
  finally { setFormBusy(form, false); }
});

$('#login-form')?.addEventListener('submit', async event => {
  event.preventDefault();
  const errorNode = $('#login-error'); errorNode.hidden = true;
  try { state.principal = await api('/auth/login', { method: 'POST', body: JSON.stringify(Object.fromEntries(new FormData(event.currentTarget))) }); state.sessionVersion += 1; $('#login-dialog').close(); applyPrincipal(); toast(`已登录：${state.principal.displayName}，Fabric 身份 ${state.principal.mspId}`); await reloadBusinessData(); }
  catch (error) { errorNode.textContent = error.message; errorNode.hidden = false; }
});

$('#register-form')?.addEventListener('submit', async event => {
  event.preventDefault();
  const formElement = event.currentTarget;
  const errorNode = $('#register-error'); errorNode.hidden = true;
  const payload = Object.fromEntries(new FormData(formElement));
  if (payload.password !== payload.confirmPassword) {
    errorNode.textContent = '两次输入的密码不一致'; errorNode.hidden = false; return;
  }
  delete payload.confirmPassword;
  try {
    state.principal = await api('/auth/register', { method: 'POST', body: JSON.stringify(payload) }); state.sessionVersion += 1;
    formElement.reset(); $('#login-dialog').close(); applyPrincipal();
    toast(`已注册 ${state.principal.displayName}，业务档案已写入 Fabric`);
    await reloadBusinessData();
  } catch (error) { errorNode.textContent = error.message; errorNode.hidden = false; }
});

$('#logout')?.addEventListener('click', async () => {
  try { await api('/auth/logout', { method: 'POST' }); } catch (error) { toast('本地已退出；服务端会话清理稍后重试。', true); }
  state.sessionVersion += 1; state.principal = null; state.invoices = []; state.organizations = []; state.registrationOrganizations = []; state.users = []; state.projects = []; state.reimbursements = []; state.editingProjectId = null; state.pendingMutationKeys.clear(); resetOCRState(true);
  ++projectInvoiceRequest; ++invoiceListRequest; window.clearTimeout(state.searchTimer);
  projectInvoiceLoadState = 'idle'; projectInvoiceLoadError = ''; state.projectInvoices = []; state.invoiceSummary = null; state.invoicePage = 1; state.invoiceQuery = ''; state.invoiceTotal = 0; state.invoiceUpdatedAt = ''; $('#search').value = ''; updateMetrics();
  $('#detail-dialog').close(); $('#project-form').reset(); $('#project-form').elements.id.disabled = false; $('#project-submit-label').textContent = '保存项目草稿'; $('#reimbursement-form').reset(); $('#invoice-form [name="issueDate"]').value = new Date().toISOString().slice(0, 10);
  applyPrincipal(); $('#invoice-rows').innerHTML = '<tr><td colspan="6" class="empty">请登录后查看账本</td></tr>'; $('#project-rows').innerHTML = '<tr><td colspan="6" class="empty">请登录后查看项目</td></tr>'; $('#reimbursement-rows').innerHTML = '<tr><td colspan="6" class="empty">请登录后查看报销单</td></tr>'; $('#organization-rows').innerHTML = '<p class="empty">请登录后查看链上组织目录</p>'; showLoginView();
});
$('#search')?.addEventListener('input', event => {
  const query = event.currentTarget.value.trim();
  const session = state.sessionVersion;
  ++invoiceListRequest;
  window.clearTimeout(state.searchTimer);
  state.searchTimer = window.setTimeout(() => {
    if (session !== state.sessionVersion || !state.principal) return;
    state.invoiceQuery = query; state.invoicePage = 1; loadInvoices();
  }, 260);
});
$('#ocr-file')?.addEventListener('change', event => { if (event.currentTarget.files[0]) setOCRFile(event.currentTarget.files[0]); });
['dragenter', 'dragover'].forEach(type => $('#ocr-drop-zone')?.addEventListener(type, event => { event.preventDefault(); event.stopPropagation(); $('#ocr-drop-zone')?.classList.add('dragging'); }));
['dragleave', 'drop'].forEach(type => $('#ocr-drop-zone')?.addEventListener(type, event => { event.preventDefault(); event.stopPropagation(); $('#ocr-drop-zone')?.classList.remove('dragging'); }));
$('#ocr-drop-zone')?.addEventListener('drop', event => {
  const file = event.dataTransfer?.files?.[0];
  if (file) setOCRFile(file);
});
$('#ocr-recognize')?.addEventListener('click', recognizeInvoiceFile);
$('#refresh')?.addEventListener('click', loadInvoices);
$('#refresh-organizations')?.addEventListener('click', loadOrganizations);
$('#refresh-projects')?.addEventListener('click', loadProjects);
$('#refresh-reimbursements')?.addEventListener('click', loadReimbursements);
$('#refresh-statistics')?.addEventListener('click', reloadBusinessData);
$('#invoice-rows')?.addEventListener('click', event => { const id = event.target.dataset.detail; if (id) openDetail(id); });
$('#project-rows')?.addEventListener('click', handleProjectAction);
$('#reimbursement-rows')?.addEventListener('click', handleReimbursementAction);
$('#invoice-project')?.addEventListener('change', updateProjectDetailButtons);
$('#reimbursement-project')?.addEventListener('change', () => { updateProjectDetailButtons(); loadProjectInvoices($('#reimbursement-project').value); });
$('#reimbursement-invoice')?.addEventListener('change', () => { const detail = $('#reimbursement-invoice-detail'); if (detail) detail.disabled = !$('#reimbursement-invoice').value; });
$('#clear-reimbursement-selection')?.addEventListener('click', () => {
  $('#reimbursement-project').value = '';
  $('#reimbursement-invoice').value = '';
  state.projectInvoices = [];
  ++projectInvoiceRequest;
  projectInvoiceLoadState = 'idle';
  updateProjectDetailButtons();
  renderReimbursementInvoiceOptions();
});
$('#reimbursement-invoice-list')?.addEventListener('click', event => {
  const selectID = event.target.dataset.reimbursementInvoiceSelect;
  const viewID = event.target.dataset.reimbursementInvoiceView;
  if (selectID) {
    $('#reimbursement-invoice').value = selectID;
    $('#reimbursement-invoice-detail').disabled = false;
    toast('已选择该发票，可继续填写材料说明并提交报销。');
  }
  if (viewID) openDetail(viewID);
});
$('#invoice-project-detail')?.addEventListener('click', () => { const project = selectedProject('#invoice-project'); if (project) openProjectDetail(project.id); });
$('#reimbursement-project-detail')?.addEventListener('click', () => { const project = selectedProject('#reimbursement-project'); if (project) openProjectDetail(project.id); });
$('#reimbursement-invoice-detail')?.addEventListener('click', () => { const id = $('#reimbursement-invoice').value; if (id) openDetail(id); });
$('#organization-type')?.addEventListener('change', updateOrganizationParentOptions);
$('#organization-rows')?.addEventListener('click', event => { const id = event.target.closest('[data-organization-detail]')?.dataset.organizationDetail; if (id) openOrganizationDetail(id); });
$('#close-dialog')?.addEventListener('click', () => $('#detail-dialog')?.close());
$('#close-alert')?.addEventListener('click', () => { const alert = $('#global-alert'); if (alert) alert.hidden = true; });
document.addEventListener('pointerdown', event => { const alert = $('#global-alert'); if (!alert.hidden && !alert.contains(event.target)) alert.hidden = true; });
$('#open-register')?.addEventListener('click', showRegister);
$('#show-register')?.addEventListener('click', showRegister);
$('#show-login')?.addEventListener('click', showLoginView);
document.querySelector('[data-view-link="dashboard"]')?.addEventListener('click', event => { event.preventDefault(); switchView('dashboard'); });
window.addEventListener('hashchange', () => switchView(location.hash.slice(1) || 'dashboard', false));
const issueDate = $('#invoice-form [name="issueDate"]');
if (issueDate) issueDate.value = new Date().toISOString().slice(0, 10);

switchView(location.hash.slice(1) || 'dashboard', false);
(async () => { try { state.principal = await api('/auth/me'); applyPrincipal(); await reloadBusinessData(); } catch (_) { applyPrincipal(); showLoginView(); } })();
