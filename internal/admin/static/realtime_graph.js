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
      for (const entry of groups.values()) { entry.label.setAttribute('y', -entry.radius + 25); entry.count.setAttribute('y', -entry.radius + 43); }
    }
    const entryFor = (kind, id) => (kind === 'room' ? groups : kind === 'node' ? nodes : edges).get(id);
    function moveRoom(entry, x, y) { positions.set(entry.data.id, { x, y }); entry.root.setAttribute('transform', 'translate(' + x + ' ' + y + ')'); }
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
      const selectedEntry = selected && entryFor(selected.kind, selected.id);
      const sessionID = data.module === 'companions' ? selectedEntry?.data.session_id : '';
      if (sessionID) for (const n of data.nodes) if (n.kind === 'user' && n.session_id === sessionID) related.add(n.id);
      if (selected?.kind === 'edge') { const edge = edges.get(selected.id)?.data; if (edge) { related.add(edge.source); related.add(edge.target); } }
      if (selected?.kind === 'node') {
        related.add(selected.id);
        for (const { data: edge } of edges.values()) if (edge.source === selected.id || edge.target === selected.id) { related.add(edge.source); related.add(edge.target); }
      }
      for (const [id, entry] of nodes) {
        const active = selected?.kind === 'node' && selected.id === id;
        entry.root.classList.toggle('is-muted', !!selected && !related.has(id)); entry.root.classList.toggle('is-selected', active);
        entry.root.setAttribute('aria-pressed', String(active));
      }
      for (const [id, entry] of edges) {
        const active = selected?.kind === 'edge' ? selected.id === id : selected?.kind === 'node' && (entry.data.source === selected.id || entry.data.target === selected.id);
        entry.root.classList.toggle('is-selected', !!active); entry.root.classList.toggle('is-muted', !!selected && !active);
        entry.hit.setAttribute('aria-pressed', String(selected?.kind === 'edge' && selected.id === id));
      }
      for (const [id, entry] of groups) {
        const active = selected?.kind === 'room' && selected.id === id;
        entry.root.classList.toggle('is-selected', active); entry.root.classList.toggle('is-muted', !!selected && entry.data.session_id !== sessionID);
        entry.root.setAttribute('aria-pressed', String(active));
      }
    }
    function select(kind, id) {
      selected = { kind, id }; highlight();
      const entry = entryFor(kind, id);
      if (entry) onSelect({ kind, value: entry.data, data });
    }
    function activate(element, handler) {
      element.addEventListener('click', event => { event.stopPropagation(); if (drag?.moved) return; handler(); });
      element.addEventListener('keydown', event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); handler(); } });
    }
    function nodeEntry(n) {
      const group = svg('g', { class: 'rt-graph-node', role: 'button', tabindex: 0 });
      const title = svg('title'), underlay = svg('circle', { r: 20, class: 'rt-graph-underlay', 'aria-hidden': true });
      const artwork = svg('g', { class: 'rt-graph-avatar' }), circle = svg('circle', { r: 20, class: 'rt-graph-surface' });
      const border = svg('circle', { r: 20, class: 'rt-graph-border', 'aria-hidden': true });
      const initial = svg('text', { y: 5, 'text-anchor': 'middle', class: 'rt-graph-initial' });
      const clipID = prefix + (++nodeNumber), clip = svg('clipPath', { id: clipID }); clip.append(svg('circle', { r: 20 })); defs.append(clip);
      const image = svg('image', { x: -20, y: -20, width: 40, height: 40, 'clip-path': 'url(#' + clipID + ')' });
      image.addEventListener('error', () => image.setAttribute('visibility', 'hidden'));
      const label = svg('text', { y: 40, 'text-anchor': 'middle', class: 'rt-graph-label' });
      const count = svg('text', { y: 56, 'text-anchor': 'middle', class: 'rt-graph-count' });
      const owner = svg('g', { class: 'rt-graph-owner', 'aria-hidden': true, visibility: 'hidden' });
      owner.append(svg('rect', { x: -17, y: 12, width: 34, height: 16, rx: 7 }), svg('text', { x: 0, y: 23, 'text-anchor': 'middle' }, '房主'));
      // Keep the backing opaque when fading the artwork so graph lines never show through an avatar.
      artwork.append(circle, initial, image); group.append(title, underlay, artwork, border, owner, label, count); nodeLayer.append(group);
      group.addEventListener('keydown', event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); select('node', n.id); } });
      group.addEventListener('pointerdown', event => {
        if (event.button !== 0) return;
        event.stopPropagation(); first = false; const p = point(event), position = positions.get(n.id);
        drag = { id: n.id, x: p.x, y: p.y, startX: event.clientX, startY: event.clientY, px: position.x, py: position.y, moved: false };
        root.setPointerCapture(event.pointerId);
      });
      return { root: group, title, initial, image, label, count, owner, clip, data: n };
    }
    function updateNode(entry, n) {
      entry.data = n;
      entry.owner.setAttribute('visibility', n.role === 'owner' ? 'visible' : 'hidden');
      entry.title.textContent = n.label + (n.role === 'owner' ? ' · 房主' : '') + '\n' + n.user_id + ' · ' + n.connections + ' 条在线连接';
      entry.root.setAttribute('aria-label', entry.title.textContent); entry.initial.textContent = Array.from(n.label || '用')[0];
      entry.label.setAttribute('y', 20 + 18 / view.scale); entry.count.setAttribute('y', 20 + 34 / view.scale);
      entry.label.textContent = Array.from(n.label || '').slice(0, 10).join('') + (Array.from(n.label || '').length > 10 ? '…' : '');
      entry.count.textContent = n.connections > 1 ? n.connections + ' 条连接' : '';
      const url = n.avatar_url || '', safe = url.startsWith('/admin/api/static/avatars/') || url.startsWith('/admin/api/static/default_avatars/');
      if (safe) {
        if (entry.image.getAttribute('href') !== url) { entry.image.setAttribute('href', url); entry.image.setAttribute('visibility', 'visible'); }
      } else { entry.image.removeAttribute('href'); entry.image.setAttribute('visibility', 'hidden'); }
    }
    function edgeEntry(edge) {
      const group = svg('g', { class: 'rt-graph-edge' }), title = svg('title'), path = svg('path', { class: 'rt-graph-line' });
      const hit = svg('path', { class: 'rt-graph-hit', role: 'button', tabindex: 0 });
      hit.append(title); group.append(hit, path); edgeLayer.append(group); activate(hit, () => select('edge', edge.id));
      return { root: group, path, hit, title, data: edge };
    }
    function roomEntry(room) {
      const group = svg('g', { class: 'rt-graph-room', role: 'button', tabindex: 0 }), title = svg('title');
      const circle = svg('circle', { class: 'rt-graph-room-region' });
      const label = svg('text', { 'text-anchor': 'middle', class: 'rt-graph-label' }), count = svg('text', { 'text-anchor': 'middle', class: 'rt-graph-count' });
      group.append(title, circle, label, count); groupLayer.append(group);
      group.addEventListener('keydown', event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); select('room', room.id); } });
      group.addEventListener('pointerdown', event => {
        if (event.button !== 0) return;
        event.stopPropagation(); first = false; const p = point(event), center = positions.get(room.id);
        drag = { kind: 'room', id: room.id, x: p.x, y: p.y, startX: event.clientX, startY: event.clientY, px: center.x, py: center.y, moved: false,
          members: data.nodes.filter(n => n.kind === 'user' && n.session_id === room.session_id).map(n => ({ id: n.id, ...positions.get(n.id) })) };
        root.setPointerCapture(event.pointerId);
      });
      return { root: group, title, circle, label, count, radius: 120, data: room };
    }
    function updateRoom(entry, room) {
      entry.data = room; entry.title.textContent = room.label + '\n' + room.session_id;
      entry.root.setAttribute('aria-label', entry.title.textContent + '，点击查看群聊与事件，可拖动整个房间');
      const text = Array.from(room.label || '同行房间'); entry.label.textContent = text.slice(0, 16).join('') + (text.length > 16 ? '…' : '');
      entry.count.textContent = data.nodes.filter(n => n.kind === 'user' && n.session_id === room.session_id).length + ' 位在线成员';
    }
    function fit() {
      const active = data.nodes.map(n => positions.get(n.id)).filter(Boolean); if (!active.length) { view = { x: 0, y: 0, scale: 1 }; transform(); return; }
      for (const entry of groups.values()) { const { x, y } = positions.get(entry.data.id), radius = entry.radius; active.push({ x: x - radius, y: y - radius }, { x: x + radius, y: y + radius }); }
      const left = Math.min(...active.map(p => p.x)) - 65, right = Math.max(...active.map(p => p.x)) + 65;
      const top = Math.min(...active.map(p => p.y)) - 55, bottom = Math.max(...active.map(p => p.y)) + 85;
      const scale = Math.min(1.3, Math.max(0.025, Math.min(width / (right - left), height / (bottom - top)) * 0.9));
      view = { scale, x: width / (2 * scale) - (left + right) / 2, y: height / (2 * scale) - (top + bottom) / 2 }; transform();
    }
    function roomLayout() {
      const layouts = data.nodes.filter(n => n.kind === 'room').map(room => {
        const members = data.nodes.filter(n => n.kind === 'user' && n.session_id === room.session_id);
        const columns = Math.max(1, Math.ceil(Math.sqrt(members.length))), rows = Math.ceil(members.length / columns);
        return { room, members, columns, rows, radius: Math.max(120, Math.hypot((columns - 1) * 54, (rows - 1) * 50) + 90) };
      });
      const step = (Math.max(120, ...layouts.map(item => item.radius)) + 45) * 2, columns = Math.max(1, Math.ceil(Math.sqrt(layouts.length * 1.3)));
      layouts.forEach(({ room, members, columns: memberColumns, rows, radius }, index) => {
        const center = { x: (index % columns) * step + step / 2, y: Math.floor(index / columns) * step + step / 2 }, entry = groups.get(room.id);
        entry.radius = radius; entry.circle.setAttribute('r', radius); moveRoom(entry, center.x, center.y);
        members.forEach((member, i) => positions.set(member.id, { x: center.x + (i % memberColumns - (memberColumns - 1) / 2) * 108, y: center.y + (Math.floor(i / memberColumns) - (rows - 1) / 2) * 100 + 12 }));
      });
      place(); transform(); if (first) fit();
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
      const topology = JSON.stringify([data.module, data.nodes.map(n => n.id).sort(), data.edges.map(e => e.id).sort()]);
      if (topology !== lastTopology) drag = null;
      const wantedNodes = new Set(data.nodes.filter(n => n.kind === 'user').map(n => n.id)), wantedRooms = new Set(data.nodes.filter(n => n.kind === 'room').map(n => n.id));
      const visibleEdges = data.module === 'messages' ? data.edges : [], wantedEdges = new Set(visibleEdges.map(e => e.id));
      const wantedPositions = new Set(data.nodes.map(n => n.id));
      if (drag?.id && !wantedPositions.has(drag.id)) drag = null;
      for (const [id, entry] of nodes) if (!wantedNodes.has(id)) { entry.root.remove(); entry.clip.remove(); nodes.delete(id); }
      for (const [id, entry] of edges) if (!wantedEdges.has(id)) { entry.root.remove(); edges.delete(id); }
      for (const [id, entry] of groups) if (!wantedRooms.has(id)) { entry.root.remove(); groups.delete(id); }
      data.nodes.forEach((n, i) => {
        if (n.kind === 'room') {
          let entry = groups.get(n.id); if (!entry) { entry = roomEntry(n); groups.set(n.id, entry); }
          updateRoom(entry, n); return;
        }
        if (!positions.has(n.id)) { const angle = i * 2.39996, radius = 65 + Math.sqrt(i) * 40; positions.set(n.id, { x: width / 2 + Math.cos(angle) * radius, y: height / 2 + Math.sin(angle) * radius }); }
        let entry = nodes.get(n.id); if (!entry) { entry = nodeEntry(n); nodes.set(n.id, entry); }
        updateNode(entry, n);
      });
      for (const edge of visibleEdges) {
        let entry = edges.get(edge.id); if (!entry) { entry = edgeEntry(edge); edges.set(edge.id, entry); }
        entry.data = edge;
        const a = nodes.get(edge.source)?.data.label || '', b = nodes.get(edge.target)?.data.label || '';
        entry.title.textContent = a + ' ↔ ' + b + (edge.last_at ? '\n最近往来：' + AdminTime.dateTime(edge.last_at) : '');
        entry.hit.setAttribute('aria-label', entry.title.textContent);
      }
      if (selected && !entryFor(selected.kind, selected.id)) { selected = null; onLost(); }
      highlight(); place();
      if (!data.nodes.length) { first = true; lastTopology = topology; return; }
      if (data.module === 'companions') { if (topology !== lastTopology || first) roomLayout(); first = false; }
      else if (topology !== lastTopology || first) settle();
      if (positions.size > 1000) for (const id of positions.keys()) if (!wantedPositions.has(id)) positions.delete(id);
      lastTopology = topology;
    }
    root.addEventListener('pointerdown', event => {
      if (event.button !== 0 || event.target.closest('.rt-graph-node,.rt-graph-hit,.rt-graph-room')) return;
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
          const room = groups.get(drag.id); if (room) moveRoom(room, drag.px + dx, drag.py + dy);
        } else {
          let x = drag.px + dx, y = drag.py + dy;
          if (data.module === 'companions') {
            const member = nodes.get(drag.id)?.data, room = member && groups.get('room:' + member.session_id), center = room && positions.get(room.data.id);
            if (center) {
              const distance = Math.hypot(x - center.x, y - center.y), limit = Math.max(0, room.radius - 65);
              if (distance > limit) { x = center.x + (x - center.x) / distance * limit; y = center.y + (y - center.y) / distance * limit; }
            }
          }
          positions.set(drag.id, { x, y });
        }
        place();
      }
    });
    root.addEventListener('pointerup', event => { const old = drag; drag = null; if (root.hasPointerCapture(event.pointerId)) root.releasePointerCapture(event.pointerId); if (old?.id && !old.moved) select(old.kind || 'node', old.id); });
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
        const n = data.nodes.find(n => n.user_id === q) || data.nodes.find(n => n.kind === 'room' && n.session_id === q) || data.nodes.find(n => n.label.toLowerCase().includes(q)); if (!n) return false;
        const p = positions.get(n.id), room = groups.get(n.id); view.scale = room ? Math.max(0.025, Math.min(1.2, width / (room.radius * 2 + 120), height / (room.radius * 2 + 120))) : 1.2;
        view.x = width / 2 / view.scale - p.x; view.y = height / 2 / view.scale - p.y; transform(); select(room ? 'room' : 'node', n.id); return true;
      },
      selection() { if (!selected) return null; return { ...selected, value: entryFor(selected.kind, selected.id)?.data, data }; },
      dispose() { disposed = true; cancelAnimationFrame(frame); observer.disconnect(); root.remove(); }
    };
  }
  window.RealtimeGraph = { create };
}());
