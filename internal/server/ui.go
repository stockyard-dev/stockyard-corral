package server

import (
	"net/http"
)

// uiHTML is the self-contained dashboard for Corral.
// Served at GET /ui — no build step, no external files.
const uiHTML = `<!DOCTYPE html><html lang="en"><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Corral — Stockyard</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Libre+Baskerville:ital,wght@0,400;0,700;1,400&family=JetBrains+Mono:wght@400;600&display=swap" rel="stylesheet">
<style>:root{
  --bg:#1a1410;--bg2:#241e18;--bg3:#2e261e;
  --rust:#c45d2c;--rust-light:#e8753a;--rust-dark:#8b3d1a;
  --leather:#a0845c;--leather-light:#c4a87a;
  --cream:#f0e6d3;--cream-dim:#bfb5a3;--cream-muted:#7a7060;
  --gold:#d4a843;--green:#5ba86e;--red:#c0392b;
  --font-serif:'Libre Baskerville',Georgia,serif;
  --font-mono:'JetBrains Mono',monospace;
}
*{margin:0;padding:0;box-sizing:border-box}
body{background:var(--bg);color:var(--cream);font-family:var(--font-serif);min-height:100vh;overflow-x:hidden}
a{color:var(--rust-light);text-decoration:none}a:hover{color:var(--gold)}
.hdr{background:var(--bg2);border-bottom:2px solid var(--rust-dark);padding:.9rem 1.8rem;display:flex;align-items:center;justify-content:space-between;gap:1rem}
.hdr-left{display:flex;align-items:center;gap:1rem}
.hdr-brand{font-family:var(--font-mono);font-size:.75rem;color:var(--leather);letter-spacing:3px;text-transform:uppercase}
.hdr-title{font-family:var(--font-mono);font-size:1.1rem;color:var(--cream);letter-spacing:1px}
.badge{font-family:var(--font-mono);font-size:.6rem;padding:.2rem .6rem;letter-spacing:1px;text-transform:uppercase;border:1px solid}
.badge-free{color:var(--green);border-color:var(--green)}
.badge-pro{color:var(--gold);border-color:var(--gold)}
.badge-ok{color:var(--green);border-color:var(--green)}
.badge-err{color:var(--red);border-color:var(--red)}
.main{max-width:1000px;margin:0 auto;padding:2rem 1.5rem}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:1rem;margin-bottom:2rem}
.card{background:var(--bg2);border:1px solid var(--bg3);padding:1.2rem 1.5rem}
.card-val{font-family:var(--font-mono);font-size:1.8rem;font-weight:700;color:var(--cream);display:block}
.card-lbl{font-family:var(--font-mono);font-size:.62rem;letter-spacing:2px;text-transform:uppercase;color:var(--leather);margin-top:.3rem}
.section{margin-bottom:2.5rem}
.section-title{font-family:var(--font-mono);font-size:.68rem;letter-spacing:3px;text-transform:uppercase;color:var(--rust-light);margin-bottom:.8rem;padding-bottom:.5rem;border-bottom:1px solid var(--bg3)}
table{width:100%;border-collapse:collapse;font-family:var(--font-mono);font-size:.78rem}
th{background:var(--bg3);padding:.5rem .8rem;text-align:left;color:var(--leather-light);font-weight:400;letter-spacing:1px;font-size:.65rem;text-transform:uppercase}
td{padding:.5rem .8rem;border-bottom:1px solid var(--bg3);color:var(--cream-dim);vertical-align:top;word-break:break-all}
tr:hover td{background:var(--bg2)}
.empty{color:var(--cream-muted);text-align:center;padding:2rem;font-style:italic}
.btn{font-family:var(--font-mono);font-size:.75rem;padding:.4rem 1rem;border:1px solid var(--leather);background:transparent;color:var(--cream);cursor:pointer;transition:all .2s}
.btn:hover{border-color:var(--rust-light);color:var(--rust-light)}
.btn-rust{border-color:var(--rust);color:var(--rust-light)}.btn-rust:hover{background:var(--rust);color:var(--cream)}
.pill{display:inline-block;font-family:var(--font-mono);font-size:.6rem;padding:.1rem .4rem;border-radius:2px;text-transform:uppercase}
.pill-get{background:#1a3a2a;color:var(--green)}.pill-post{background:#2a1f1a;color:var(--rust-light)}
.pill-del{background:#2a1a1a;color:var(--red)}.pill-ok{background:#1a3a2a;color:var(--green)}
.pill-err{background:#2a1a1a;color:var(--red)}
.mono{font-family:var(--font-mono);font-size:.78rem}
.lbl{font-family:var(--font-mono);font-size:.62rem;letter-spacing:1px;text-transform:uppercase;color:var(--leather)}
.upgrade{background:var(--bg2);border:1px solid var(--rust-dark);border-left:3px solid var(--rust);padding:.8rem 1.2rem;font-size:.82rem;color:var(--cream-dim);margin-bottom:1.5rem}
.upgrade a{color:var(--rust-light)}
pre{background:var(--bg3);padding:.8rem 1rem;font-family:var(--font-mono);font-size:.75rem;color:var(--cream-dim);overflow-x:auto;max-width:100%}
input,select{font-family:var(--font-mono);font-size:.78rem;background:var(--bg3);border:1px solid var(--bg3);color:var(--cream);padding:.4rem .7rem;outline:none}
input:focus,select:focus{border-color:var(--leather)}
.row{display:flex;gap:.8rem;align-items:flex-end;flex-wrap:wrap;margin-bottom:1rem}
.field{display:flex;flex-direction:column;gap:.3rem}
.sserow{padding:.4rem .8rem;border-bottom:1px solid var(--bg3);font-family:var(--font-mono);font-size:.72rem;color:var(--cream-dim);display:grid;grid-template-columns:120px 60px 1fr;gap:.5rem}
.sserow:nth-child(odd){background:var(--bg2)}
</style></head><body>
<div class="hdr">
  <div class="hdr-left">
    <svg viewBox="0 0 64 64" width="22" height="22" fill="none"><rect x="8" y="8" width="8" height="48" rx="2.5" fill="#e8753a"/><rect x="28" y="8" width="8" height="48" rx="2.5" fill="#e8753a"/><rect x="48" y="8" width="8" height="48" rx="2.5" fill="#e8753a"/><rect x="8" y="27" width="48" height="7" rx="2.5" fill="#c4a87a"/></svg>
    <span class="hdr-brand">Stockyard</span>
    <span class="hdr-title">Corral</span>
  </div>
  <div style="display:flex;gap:.8rem;align-items:center">
    <span id="tier-badge" class="badge badge-free">Free</span>
    <a href="/api/stats" class="lbl" style="color:var(--leather)">API</a>
    <a href="https://stockyard.dev/corral/" class="lbl" style="color:var(--leather)">Docs</a>
  </div>
</div>
<div class="main">

<div id="upgrade-banner" class="upgrade" style="display:none">
  <strong style="color:var(--cream)">Free tier</strong> — 3 endpoints, 1K events/mo, 7-day retention.
  <a href="https://stockyard.dev/corral/" target="_blank">Upgrade to Pro for $0.99/mo →</a>
</div>

<div class="cards" id="stat-cards">
  <div class="card"><span class="card-val" id="s-eps">—</span><span class="card-lbl">Endpoints</span></div>
  <div class="card"><span class="card-val" id="s-evts">—</span><span class="card-lbl">Events</span></div>
  <div class="card"><span class="card-val" id="s-rpl">—</span><span class="card-lbl">Replays</span></div>
  <div class="card"><span class="card-val" id="s-fwd">—</span><span class="card-lbl">Forward Rules</span></div>
</div>

<div class="section">
  <div class="section-title">Endpoints</div>
  <div class="row">
    <div class="field"><span class="lbl">Name</span><input id="ep-name" placeholder="my-webhook" style="width:180px"></div>
    <button class="btn btn-rust" onclick="createEndpoint()">+ Create</button>
  </div>
  <table><thead><tr><th>ID</th><th>Name</th><th>Events</th><th>Hook URL</th><th>Created</th><th></th></tr></thead>
  <tbody id="ep-list"><tr><td colspan="6" class="empty">Loading...</td></tr></tbody></table>
</div>

<div class="section">
  <div class="section-title">Recent Events</div>
  <table><thead><tr><th>ID</th><th>Endpoint</th><th>Method</th><th>Path</th><th>Size</th><th>Received</th><th></th></tr></thead>
  <tbody id="evt-list"><tr><td colspan="7" class="empty">Select an endpoint above to see events, or wait for auto-refresh.</td></tr></tbody></table>
</div>

<div class="section" id="payload-section" style="display:none">
  <div class="section-title">Event Detail — <span id="payload-id" class="mono"></span></div>
  <div style="margin-bottom:.8rem;display:flex;gap:.8rem;align-items:center">
    <button class="btn btn-rust" id="replay-btn" onclick="replayEvent()">Replay</button>
    <input id="replay-target" placeholder="http://localhost:3000/webhook" style="width:280px">
    <span id="replay-result" class="mono" style="color:var(--green)"></span>
  </div>
  <pre id="payload-body" style="max-height:300px;overflow:auto"></pre>
</div>

</div>
<script>
let _timer=null;
function autoReload(fn,ms=8000){if(_timer)clearInterval(_timer);_timer=setInterval(fn,ms)}
function ts(s){if(!s)return'-';const d=new Date(s);return d.toLocaleString()}
function rel(s){if(!s)return'-';const d=new Date(s),n=new Date(),diff=Math.round((n-d)/1000);if(diff<60)return diff+'s ago';if(diff<3600)return Math.round(diff/60)+'m ago';return Math.round(diff/3600)+'h ago'}
function fmt(n){return n===undefined||n===null?'-':n.toLocaleString()}
function pill(m){const c={'GET':'pill-get','POST':'pill-post','DELETE':'pill-del'}[m]||'';return '<span class="pill '+c+'">'+m+'</span>'}
function status(s){const ok=s>=200&&s<300;return '<span class="pill '+(ok?'pill-ok':'pill-err')+'">'+s+'</span>'}

const API='/api';
let activeEp=null,activeEvt=null;

async function loadStats(){
  const s=await(await fetch(API+'/status')).json().catch(()=>({}));
  document.getElementById('s-eps').textContent=fmt(s.endpoints);
  document.getElementById('s-evts').textContent=fmt(s.events);
  document.getElementById('s-rpl').textContent=fmt(s.replays);
  document.getElementById('s-fwd').textContent=fmt(s.forward_rules);
}

async function loadEndpoints(){
  const r=await(await fetch(API+'/endpoints')).json().catch(()=>({endpoints:[]}));
  const eps=r.endpoints||[];
  document.getElementById('ep-list').innerHTML=eps.length?eps.map(e=>
    ` + "`" + `<tr>
      <td><span class="mono" style="color:var(--leather-light)">${e.id}</span></td>
      <td style="color:var(--cream)">${e.name}</td>
      <td>${e.event_count||0}</td>
      <td><span class="mono" style="font-size:.7rem;color:var(--rust-light)">/hook/${e.id}</span></td>
      <td>${rel(e.created_at)}</td>
      <td><button class="btn" onclick="loadEvents('${e.id}')">Events</button></td>
    </tr>` + "`" + `).join(''):'<tr><td colspan="6" class="empty">No endpoints yet — create one above.</td></tr>';
}

async function loadEvents(epId){
  activeEp=epId;
  const r=await(await fetch(API+'/endpoints/'+epId+'/events?limit=50')).json().catch(()=>({events:[]}));
  const evts=r.events||[];
  document.getElementById('evt-list').innerHTML=evts.length?evts.map(e=>
    ` + "`" + `<tr>
      <td><span class="mono" style="color:var(--leather-light);font-size:.7rem">${e.id}</span></td>
      <td class="mono" style="font-size:.7rem">${epId}</td>
      <td>${pill(e.method||'POST')}</td>
      <td class="mono" style="font-size:.72rem;max-width:200px;overflow:hidden;text-overflow:ellipsis">${e.path||'/'}</td>
      <td>${e.body_size||0}B</td>
      <td>${rel(e.received_at)}</td>
      <td><button class="btn" onclick="showPayload('${e.id}')">Inspect</button></td>
    </tr>` + "`" + `).join(''):'<tr><td colspan="7" class="empty">No events for this endpoint yet.</td></tr>';
}

async function showPayload(evtId){
  activeEvt=evtId;
  const r=await(await fetch(API+'/events/'+evtId)).json().catch(()=>({}));
  const e=r.event||r;
  document.getElementById('payload-section').style.display='block';
  document.getElementById('payload-id').textContent=evtId;
  let body=e.body||'';
  try{body=JSON.stringify(JSON.parse(body),null,2)}catch(x){}
  document.getElementById('payload-body').textContent=
    'Method: '+(e.method||'POST')+'\nPath: '+(e.path||'/')+'\nContent-Type: '+(e.content_type||'')+'\n\n'+body;
}

async function replayEvent(){
  if(!activeEvt)return;
  const target=document.getElementById('replay-target').value.trim();
  if(!target)return;
  const r=await fetch(API+'/events/'+activeEvt+'/replay',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({target})}).catch(()=>null);
  if(!r){document.getElementById('replay-result').textContent='fetch error';return;}
  const j=await r.json().catch(()=>({}));
  if(r.status===402){document.getElementById('replay-result').style.color='var(--gold)';document.getElementById('replay-result').textContent='Pro required';return;}
  const ok=j.status_code>=200&&j.status_code<300;
  document.getElementById('replay-result').style.color=ok?'var(--green)':'var(--red)';
  document.getElementById('replay-result').textContent=(j.status_code||'err')+' in '+(j.latency_ms||0)+'ms';
}

async function createEndpoint(){
  const name=document.getElementById('ep-name').value.trim()||('ep-'+Date.now());
  const r=await fetch(API+'/endpoints',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name})}).catch(()=>null);
  if(!r)return;
  if(r.status===402){alert('Free tier: 3 endpoint limit reached. Upgrade to Pro at stockyard.dev/corral/');return;}
  document.getElementById('ep-name').value='';
  loadEndpoints();
}

async function refresh(){await Promise.all([loadStats(),loadEndpoints()]);if(activeEp)loadEvents(activeEp);}
refresh();autoReload(refresh,6000);
fetch(API+'/tier').then(r=>r.json()).then(j=>{if(j.tier==='free'){document.getElementById('upgrade-banner').style.display='block';document.getElementById('tier-badge').className='badge badge-free';document.getElementById('tier-badge').textContent='Free'}else{document.getElementById('tier-badge').className='badge badge-pro';document.getElementById('tier-badge').textContent='Pro'}}).catch(()=>{document.getElementById('upgrade-banner').style.display='block'});
</script></body></html>`

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(uiHTML))
}
