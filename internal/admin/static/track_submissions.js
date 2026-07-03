(function () {
  var selected = null;
  function esc(v) { return String(v == null ? '' : v).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;'); }
  async function api(path, options) { var r = await fetch(path, options); var j = await r.json(); if (!r.ok) throw new Error(j.error || ('HTTP '+r.status)); return j.data; }
  api('/admin/api/me').then(function (d) { document.getElementById('userName').textContent = '👤 '+d.username; }).catch(function(){ location.href='/admin/login.html'; });
  document.getElementById('logoutBtn').onclick = async function(){ await fetch('/admin/api/logout',{method:'POST'}); location.href='/admin/login.html'; };
  async function load() {
    var status = document.getElementById('status').value;
    var data = await api('/admin/api/track-submissions?limit=100&status='+encodeURIComponent(status));
    var list = document.getElementById('list'); list.innerHTML='';
    (data.items||[]).forEach(function(s){ var tr=document.createElement('tr'); tr.innerHTML='<td>'+esc(s.submission_id)+'</td><td>'+esc(s.track_id)+'</td><td>'+esc(s.title)+'</td><td>'+esc(s.track_type)+'</td><td>'+esc(s.difficulty)+'</td><td>'+esc(s.risk_level)+'</td><td>'+esc(s.status)+'</td><td>'+esc(s.revision)+'</td><td>'+esc((s.submitted_at||'').replace('T',' ').slice(0,19))+'</td><td><button>查看</button></td>'; tr.querySelector('button').onclick=function(){show(s.submission_id);}; list.appendChild(tr); });
  }
  async function show(id) {
    var data=await api('/admin/api/track-submissions/'+encodeURIComponent(id)); selected=data.submission;
    document.getElementById('detailPanel').hidden=false; document.getElementById('detailTitle').textContent=selected.title;
    var imgs=(selected.images||[]).map(function(i){return i.url?'<a href="'+esc(i.url)+'" target="_blank"><img src="'+esc(i.url)+'" style="max-width:180px;max-height:120px;margin:4px"></a>':'';}).join('');
    document.getElementById('detail').innerHTML='<p>'+esc(selected.description)+'</p><p>难度：'+esc(selected.difficulty)+'；风险：'+esc(selected.risk_level)+'；适宜月份：'+esc((selected.suitable_months||[]).join(','))+'</p><p>路面地形：'+esc((selected.surface_types||[]).join(','))+'</p><p>交通：'+esc((selected.transport_modes||[]).join(','))+'；'+esc(selected.transport_description)+'</p><div>'+imgs+'</div>';
  }
  async function review(decision){ if(!selected)return; var reason=document.getElementById('reviewReason').value.trim(); try{await api('/admin/api/track-submissions/'+encodeURIComponent(selected.submission_id)+'/review',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({decision:decision,reason:reason,expected_revision:selected.revision})}); selected=null; document.getElementById('detailPanel').hidden=true; await load();}catch(e){alert(e.message);} }
  document.getElementById('approveBtn').onclick=function(){review('approved');}; document.getElementById('rejectBtn').onclick=function(){review('rejected');}; document.getElementById('refreshBtn').onclick=load; document.getElementById('status').onchange=load; load();
})();
