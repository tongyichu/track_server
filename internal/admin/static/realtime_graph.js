(function () {
  'use strict';
  const NS = 'http://www.w3.org/2000/svg';
  let instance = 0;
  function svg(tag, attributes = {}, text) {
    const node = document.createElementNS(NS, tag);
    for (const [name, value] of Object.entries(attributes)) node.setAttribute(name, String(value));
    if (text != null) node.textContent = text;
    return node;
  }
  function create(host, { onSelect, onLost = () => {} }) {
    const prefix = 'rt-graph-' + (++instance) + '-';
    const root = svg('svg', { role: 'group', 'aria-label': '在线用户通信关系图，可拖动节点、拖动画布或缩放' });
    const defs = svg('defs'), scene = svg('g'), groupLayer = svg('g'), edgeLayer = svg('g'), nodeLayer = svg('g');
    scene.append(groupLayer, edgeLayer, nodeLayer); root.append(defs, scene); host.append(root);
    const positions = new Map(), nodes = new Map(), edges = new Map(), groups = new Map();
    let data = { nodes: [], edges: [], module: 'messages' }, selected = null, view = { x: 0, y: 0, scale: 1 }, width = 800, height = 520;
    let drag = null, frame = null, disposed = false, lastTopology = '', first = true, nodeNumber = 0;
    const point = event => { const rect = root.getBoundingClientRect(); return { x: (event.clientX - rect.left) / view.scale - view.x, y: (event.clientY - rect.top) / view.scale - view.y }; };
    function transform() {
      scene.setAttribute('transform', 'scale(' + view.scale + ') translate(' + view.x + ' ' + view.y + ')');
      root.style.setProperty('--rt-graph-scale', String(view.scale));
      for (const entry of nodes.values()) { entry.label.setAttribute('y', 20 + 18 / view.scale); entry.count.setAttribute('y', 20 + 34 / view.scale); }
    }
    function place() {
      for (const [id, entry] of nodes) { const p = positions.get(id); if (p) entry.root.setAttribute('transform', 'translate(' + p.x + ' ' + p.y + ')'); }
      for (const entry of edges.values()) {
        const a = positions.get(entry.data.source), b = positions.get(entry.data.target); if (!a || !b) continue;
        const path = 'M ' + a.x + ' ' + a.y + ' L ' + b.x + ' ' + b.y;
        entry.path.setAttribute('d', path); entry.hit.setAttribute('d', path);
      }
    }
    function highlight() {
      const related = new Set();
      if (selected?.kind === 'edge') { const edge = edges.get(selected.id)?.data; if (edge) { related.add(edge.source); related.add(edge.target); } }
      if (selected?.kind === 'node') {
        related.add(selected.id);
        for (const { data: edge } of edges.values()) if (edge.source === selected.id || edge.target === selected.id) { related.add(edge.source); related.add(edge.target); }
      }
      for (const [id, entry] of nodes) { entry.root.classList.toggle('is-muted', !!selected && !related.has(id)); entry.root.classList.toggle('is-selected', related.has(id)); }
      for (const [id, entry] of edges) {
        const active = selected?.kind === 'edge' ? selected.id === id : selected?.kind === 'node' && (entry.data.source === selected.id || entry.data.target === selected.id);
        entry.root.classList.toggle('is-selected', !!active); entry.root.classList.toggle('is-muted', !!selected && !active);
      }
    }
    function select(kind, id) {
      selected = { kind, id }; highlight();
      const entry = kind === 'node' ? nodes.get(id) : edges.get(id);
      if (entry) onSelect({ kind, value: entry.data, data });
    }
    function activate(element, handler) {
      element.addEventListener('click', event => { event.stopPropagation(); if (drag?.moved) return; handler(); });
      element.addEventListener('keydown', event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); handler(); } });
    }
    function nodeEntry(n) {
      const group = svg('g', { class: 'rt-graph-node', role: 'button', tabindex: 0 });
      const title = svg('title'), halo = svg('circle', { r: 25, class: 'rt-graph-halo' }), circle = svg('circle', { r: 20, class: 'rt-graph-surface' });
      const initial = svg('text', { y: 5, 'text-anchor': 'middle', class: 'rt-graph-initial' });
      const clipID = prefix + (++nodeNumber), clip = svg('clipPath', { id: clipID }); clip.append(svg('circle', { r: 20 })); defs.append(clip);
      const image = svg('image', { x: -20, y: -20, width: 40, height: 40, 'clip-path': 'url(#' + clipID + ')' });
      image.addEventListener('error', () => image.setAttribute('visibility', 'hidden'));
      const label = svg('text', { y: 40, 'text-anchor': 'middle', class: 'rt-graph-label' });
      const count = svg('text', { y: 56, 'text-anchor': 'middle', class: 'rt-graph-count' });
      group.append(title, halo, circle, initial, image, label, count); nodeLayer.append(group);
      group.addEventListener('keydown', event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); select('node', n.id); } });
      group.addEventListener('pointerdown', event => {
        if (event.button !== 0) return;
        event.stopPropagation(); first = false; const p = point(event), position = positions.get(n.id);
        drag = { id: n.id, x: p.x, y: p.y, startX: event.clientX, startY: event.clientY, px: position.x, py: position.y, moved: false };
        if (n.kind === 'room') {
          drag.members = data.nodes.filter(member => member.session_id === n.session_id).map(member => ({ id: member.id, ...positions.get(member.id) }));
          const region = groups.get(n.id); if (region) drag.region = { node: region, x: Number(region.getAttribute('cx')), y: Number(region.getAttribute('cy')) };
        }
        root.setPointerCapture(event.pointerId);
      });
      return { root: group, title, initial, image, label, count, clip, data: n };
    }
    function updateNode(entry, n) {
      entry.data = n; const room = n.kind === 'room';
      entry.root.classList.toggle('is-room', room);
      entry.title.textContent = n.label + '\n' + (room ? n.session_id : n.user_id + ' · ' + n.connections + ' 条在线连接');
      entry.root.setAttribute('aria-label', entry.title.textContent); entry.initial.textContent = room ? '房' : Array.from(n.label || '用')[0];
      entry.label.setAttribute('y', 20 + 18 / view.scale); entry.count.setAttribute('y', 20 + 34 / view.scale);
      entry.label.textContent = Array.from(n.label || '').slice(0, 10).join('') + (Array.from(n.label || '').length > 10 ? '…' : '');
      entry.count.textContent = room ? data.nodes.filter(item => item.kind === 'user' && item.session_id === n.session_id).length + ' 位在线成员' : n.connections > 1 ? n.connections + ' 条连接' : '';
      const url = n.avatar_url || '', safe = url.startsWith('/admin/api/static/avatars/') || url.startsWith('/admin/api/static/default_avatars/');
      if (safe && !room) {
        if (entry.image.getAttribute('href') !== url) { entry.image.setAttribute('href', url); entry.image.setAttribute('visibility', 'visible'); }
      } else { entry.image.removeAttribute('href'); entry.image.setAttribute('visibility', 'hidden'); }
    }
    function edgeEntry(edge) {
      const group = svg('g', { class: 'rt-graph-edge' }), title = svg('title'), path = svg('path', { class: 'rt-graph-line' });
      const hit = svg('path', { class: 'rt-graph-hit', role: 'button', tabindex: 0 });
      hit.append(title); group.append(path, hit); edgeLayer.append(group); activate(hit, () => select('edge', edge.id));
      return { root: group, path, hit, title, data: edge };
    }
    function fit() {
      const active = data.nodes.map(n => positions.get(n.id)).filter(Boolean); if (!active.length) { view = { x: 0, y: 0, scale: 1 }; transform(); return; }
      for (const region of groups.values()) { const x = Number(region.getAttribute('cx')), y = Number(region.getAttribute('cy')), radius = Number(region.getAttribute('r')); active.push({ x: x - radius, y: y - radius }, { x: x + radius, y: y + radius }); }
      const left = Math.min(...active.map(p => p.x)) - 65, right = Math.max(...active.map(p => p.x)) + 65;
      const top = Math.min(...active.map(p => p.y)) - 55, bottom = Math.max(...active.map(p => p.y)) + 85;
      const scale = Math.min(1.3, Math.max(0.025, Math.min(width / (right - left), height / (bottom - top)) * 0.9));
      view = { scale, x: width / (2 * scale) - (left + right) / 2, y: height / (2 * scale) - (top + bottom) / 2 }; transform();
    }
    function roomLayout() {
      const rooms = data.nodes.filter(n => n.kind === 'room'), radiusMax = Math.max(100, ...rooms.map(room => data.nodes.filter(n => n.kind === 'user' && n.session_id === room.session_id).length * 15)), step = (radiusMax + 90) * 2, columns = Math.max(1, Math.ceil(Math.sqrt(rooms.length * 1.3)));
      const wanted = new Set();
      rooms.forEach((room, index) => {
        const members = data.nodes.filter(n => n.kind === 'user' && n.session_id === room.session_id);
        const radius = Math.max(100, members.length * 15), center = { x: (index % columns) * step + step / 2, y: Math.floor(index / columns) * step + step / 2 };
        positions.set(room.id, center);
        members.forEach((member, i) => { const angle = i * Math.PI * 2 / Math.max(1, members.length) - Math.PI / 2; positions.set(member.id, { x: center.x + Math.cos(angle) * radius, y: center.y + Math.sin(angle) * radius }); });
        let region = groups.get(room.id);
        if (!region) { region = svg('circle', { class: 'rt-graph-room-region' }); groups.set(room.id, region); groupLayer.append(region); }
        region.setAttribute('cx', center.x); region.setAttribute('cy', center.y); region.setAttribute('r', radius + 70); wanted.add(room.id);
      });
      for (const [id, region] of groups) if (!wanted.has(id)) { region.remove(); groups.delete(id); }
      place(); if (first) fit();
    }
    function settle() {
      let ticks = 0; const ids = data.nodes.map(n => n.id);
      const animated = !window.matchMedia('(prefers-reduced-motion: reduce)').matches;
      function step() {
        if (disposed || data.module !== 'messages') return;
        const forces = new Map(ids.map(id => [id, { x: 0, y: 0 }]));
        for (let i = 0; i < ids.length; i++) for (let j = i + 1; j < ids.length; j++) {
          const a = positions.get(ids[i]), b = positions.get(ids[j]); if (!a || !b) continue;
          let dx = a.x - b.x, dy = a.y - b.y; if (Math.abs(dx) + Math.abs(dy) < 0.01) dx = 1;
          const distance = Math.max(20, Math.hypot(dx, dy)), force = Math.min(10, 1700 / (distance * distance));
          const x = dx / distance * force, y = dy / distance * force;
          forces.get(ids[i]).x += x; forces.get(ids[i]).y += y; forces.get(ids[j]).x -= x; forces.get(ids[j]).y -= y;
        }
        for (const edge of data.edges) {
          const a = positions.get(edge.source), b = positions.get(edge.target); if (!a || !b) continue;
          const dx = b.x - a.x, dy = b.y - a.y, distance = Math.max(1, Math.hypot(dx, dy)), force = (distance - 165) * 0.018;
          const x = dx / distance * force, y = dy / distance * force;
          forces.get(edge.source).x += x; forces.get(edge.source).y += y; forces.get(edge.target).x -= x; forces.get(edge.target).y -= y;
        }
        for (const id of ids) {
          if (drag?.id === id) continue;
          const p = positions.get(id), f = forces.get(id); if (!p || !f) continue;
          p.x += Math.max(-10, Math.min(10, f.x + (width / 2 - p.x) * 0.002)); p.y += Math.max(-10, Math.min(10, f.y + (height / 2 - p.y) * 0.002));
        }
        place(); ticks++;
        if (ticks < 100 && !drag) { if (animated) frame = requestAnimationFrame(step); else step(); }
        else { frame = null; if (first) { fit(); first = false; } }
      }
      frame = requestAnimationFrame(step);
    }
    function setData(next) {
      cancelAnimationFrame(frame); const changedMode = next.module !== data.module;
      data = next; if (changedMode) { first = true; selected = null; onLost(); }
      const wantedNodes = new Set(data.nodes.map(n => n.id)), wantedEdges = new Set(data.edges.map(e => e.id));
      if (drag?.id && !wantedNodes.has(drag.id)) drag = null;
      for (const [id, entry] of nodes) if (!wantedNodes.has(id)) { entry.root.remove(); entry.clip.remove(); nodes.delete(id); }
      for (const [id, entry] of edges) if (!wantedEdges.has(id)) { entry.root.remove(); edges.delete(id); }
      data.nodes.forEach((n, i) => {
        if (!positions.has(n.id)) { const angle = i * 2.39996, radius = 65 + Math.sqrt(i) * 40; positions.set(n.id, { x: width / 2 + Math.cos(angle) * radius, y: height / 2 + Math.sin(angle) * radius }); }
        let entry = nodes.get(n.id); if (!entry) { entry = nodeEntry(n); nodes.set(n.id, entry); }
        updateNode(entry, n);
      });
      for (const edge of data.edges) {
        let entry = edges.get(edge.id); if (!entry) { entry = edgeEntry(edge); edges.set(edge.id, entry); }
        entry.data = edge;
        const a = nodes.get(edge.source)?.data.label || '', b = nodes.get(edge.target)?.data.label || '';
        entry.title.textContent = a + ' ↔ ' + b + (edge.last_at ? '\n最近往来：' + AdminTime.dateTime(edge.last_at) : '');
        entry.hit.setAttribute('aria-label', entry.title.textContent);
      }
      if (selected && !(selected.kind === 'node' ? nodes.has(selected.id) : edges.has(selected.id))) { selected = null; onLost(); }
      highlight(); place();
      const topology = JSON.stringify([data.module, data.nodes.map(n => n.id).sort(), data.edges.map(e => e.id).sort()]);
      if (!data.nodes.length) { for (const region of groups.values()) region.remove(); groups.clear(); first = true; lastTopology = topology; return; }
      if (data.module === 'companions') { if (topology !== lastTopology || first) roomLayout(); first = false; }
      else if (topology !== lastTopology || first) { for (const region of groups.values()) region.remove(); groups.clear(); settle(); }
      if (positions.size > 1000) for (const id of positions.keys()) if (!wantedNodes.has(id)) positions.delete(id);
      lastTopology = topology;
    }
    root.addEventListener('pointerdown', event => {
      if (event.button !== 0 || event.target.closest('.rt-graph-node,.rt-graph-hit')) return;
      first = false; drag = { pan: true, x: event.clientX, y: event.clientY, vx: view.x, vy: view.y, moved: false }; root.setPointerCapture(event.pointerId);
    });
    root.addEventListener('pointermove', event => {
      if (!drag) return;
      if (drag.pan) { const dx = event.clientX - drag.x, dy = event.clientY - drag.y; drag.moved ||= Math.hypot(dx, dy) > 4; view.x = drag.vx + dx / view.scale; view.y = drag.vy + dy / view.scale; transform(); }
      else {
        const p = point(event), dx = p.x - drag.x, dy = p.y - drag.y;
        drag.moved ||= Math.hypot(event.clientX - drag.startX, event.clientY - drag.startY) > 4;
        if (drag.members) {
          for (const member of drag.members) if (nodes.has(member.id)) positions.set(member.id, { x: member.x + dx, y: member.y + dy });
          if (drag.region) { drag.region.node.setAttribute('cx', drag.region.x + dx); drag.region.node.setAttribute('cy', drag.region.y + dy); }
        } else positions.set(drag.id, { x: drag.px + dx, y: drag.py + dy });
        place();
      }
    });
    root.addEventListener('pointerup', event => { const old = drag; drag = null; if (root.hasPointerCapture(event.pointerId)) root.releasePointerCapture(event.pointerId); if (old?.id && !old.moved) select('node', old.id); });
    root.addEventListener('pointercancel', () => { drag = null; });
    root.addEventListener('wheel', event => {
      event.preventDefault(); first = false; const rect = root.getBoundingClientRect(), x = event.clientX - rect.left, y = event.clientY - rect.top;
      const next = Math.max(0.025, Math.min(3, view.scale * (event.deltaY > 0 ? 0.9 : 1.1)));
      view.x += x / next - x / view.scale; view.y += y / next - y / view.scale; view.scale = next; transform();
    }, { passive: false });
    const observer = new ResizeObserver(() => { const oldWidth = width, oldHeight = height; width = Math.max(260, host.clientWidth); height = Math.max(360, host.clientHeight); view.x += (width - oldWidth) / 2 / view.scale; view.y += (height - oldHeight) / 2 / view.scale; transform(); root.setAttribute('viewBox', '0 0 ' + width + ' ' + height); if (first) fit(); }); observer.observe(host);
    return {
      setData, fit() { first = false; fit(); },
      zoom(factor) { first = false; const scale = Math.max(0.025, Math.min(3, view.scale * factor)); view.x += width / 2 / scale - width / 2 / view.scale; view.y += height / 2 / scale - height / 2 / view.scale; view.scale = scale; transform(); },
      pause() { cancelAnimationFrame(frame); frame = null; },
      clearSelection() { selected = null; highlight(); },
      find(query) {
        const q = query.trim().toLowerCase(); if (!q) return false;
        const n = data.nodes.find(n => n.user_id === q || n.session_id === q || n.label.toLowerCase().includes(q)); if (!n) return false;
        const p = positions.get(n.id); view.scale = 1.2; view.x = width / 2 / view.scale - p.x; view.y = height / 2 / view.scale - p.y; transform(); select('node', n.id); return true;
      },
      selection() { if (!selected) return null; return { ...selected, value: (selected.kind === 'node' ? nodes : edges).get(selected.id)?.data, data }; },
      dispose() { disposed = true; cancelAnimationFrame(frame); observer.disconnect(); root.remove(); }
    };
  }
  window.RealtimeGraph = { create };
}());
