// Run from project root: gjs scripts/test-ui-regressions.js
// Exercises production functions with a small DOM/network test double.
const Gio = imports.gi.Gio;
const [, bytes] = Gio.File.new_for_path('web/app.js').load_contents(null);
const source = imports.byteArray.toString(bytes);
function extract(name) {
  const match = source.match(new RegExp('^(?:async )?function ' + name + '\\([\\s\\S]*?^}', 'm'));
  if (!match) throw new Error('Missing production function: ' + name);
  return match[0];
}
function assert(value, message) { if (!value) throw new Error(message); }
async function run() {
  const nodes = {};
  const $ = id => nodes[id] || (nodes[id] = {value: '', innerHTML: '', disabled: false});
  const state = {principal: {username: 'alice'}, sessionVersion: 1, projectInvoices: [], reimbursements: [{invoiceId:'i',status:'WITHDRAWN'}]};
  $('#reimbursement-project').value = 'p';
  state.projectInvoices = [{id:'i',projectId:'p',invoiceNo:'123',status:'ISSUED',currency:'CNY',totalCents:100}];
  const render = new Function('state', '$', 'safe', 'money', `let projectInvoiceLoadState='ready'; let projectInvoiceLoadError=''; const activeReimbursements=()=>state.reimbursements.filter(item=>item.status!=='WITHDRAWN'); ${extract('renderReimbursementInvoiceOptions')} return renderReimbursementInvoiceOptions;`)(state,$,String,String);
  render();
  assert($('#reimbursement-invoice').innerHTML.includes('value="i"'), 'withdrawn invoice must be selectable');
  state.reimbursements[0].status = 'PAID'; render();
  assert(!$('#reimbursement-invoice').innerHTML.includes('value="i"'), 'paid invoice must stay occupied');

  const pending = {};
  const load = new Function('state', '$', 'api', 'renderReimbursementInvoiceOptions', `let projectInvoiceRequest=0, projectInvoiceLoadState='idle', projectInvoiceLoadError=''; ${extract('loadProjectInvoices')} return loadProjectInvoices;`)(state,$, path=>new Promise(resolve=>{pending[path]=resolve;}),()=>{});
  $('#reimbursement-project').value='old'; const old=load('old');
  $('#reimbursement-project').value='new'; const recent=load('new');
  pending['/projects/new/invoices']([{id:'new'}]); await recent;
  pending['/projects/old/invoices']([{id:'old'}]); await old;
  assert(state.projectInvoices[0].id==='new', 'late old response overwrote current project');
  const loggedOut=load('new'); state.sessionVersion++;
  pending['/projects/new/invoices']([{id:'secret'}]); await loggedOut;
  assert(state.projectInvoices.length===0, 'previous session response leaked into new session');

  state.invoiceSummary={total:45,circulating:23,amountCents:789}; state.invoices=[];
  new Function('state','$','money',extract('updateMetrics')+';updateMetrics();')(state,$,String);
  assert($('#metric-circulating').textContent===23, 'overview must not count only current page');

  const form={}; const event={preventDefault(){},currentTarget:form}; const calls=[];
  const submit=new Function('api','FormData','setFormBusy','showDetailError',extract('resubmitReimbursement')+';return resubmitReimbursement;')(
    async()=>{event.currentTarget=null;throw new Error('expected failure');},
    class {get(){return 'evidence';}},(target,busy)=>calls.push([target,busy]),()=>{});
  await submit(event,'r');
  assert(calls.length===2 && calls[1][0]===form && calls[1][1]===false,'failed async submit must re-enable original form');
  print('PASS: withdrawal availability, paid invoice exclusion, request ordering, session isolation, global metrics, async form recovery');
}
const loop=new imports.gi.GLib.MainLoop(null,false);
let failed=false;
run().catch(error=>{printerr(error.stack);failed=true;}).finally(()=>loop.quit());
loop.run();
if(failed) imports.system.exit(1);
