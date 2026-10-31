(function () {
  // 通用：登录态校验 + 退出
  fetch('/admin/api/me').then(function (r) {
    if (!r.ok) { window.location.href = '/admin/login.html'; return null; }
    return r.json();
  }).then(function (j) {
    if (j && j.data) {
      var el = document.getElementById('userName');
      if (el) el.textContent = '👤 ' + j.data.username;
    }
  });
  document.getElementById('logoutBtn').addEventListener('click', async function () {
    await fetch('/admin/api/logout', { method: 'POST' });
    window.location.href = '/admin/login.html';
  });

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  function fmtTime(s) {
    return esc((s || '').replace('T', ' ').slice(0, 19));
  }
  function restrictionText(r) {
    if (!r) return '<span class="badge bg-green-lt text-green">正常</span>';
    var until = r.expires_at ? ('至 ' + fmtTime(r.expires_at)) : '永久';
    return '<span class="badge bg-red-lt text-red">已限制</span><br><span class="hint">' + esc(until) + '</span><br><span class="hint">' + esc(r.reason || '') + '</span>';
  }
  function genderText(value) {
    return value === 'male' ? '男' : (value === 'female' ? '女' : '-');
  }
  function zodiacText(value) {
    return distributionLabels[value] || '-';
  }

  var distributionLabels = {
    male: '男', female: '女', unknown: '未知', aries: '白羊座', taurus: '金牛座', gemini: '双子座',
    cancer: '巨蟹座', leo: '狮子座', virgo: '处女座', libra: '天秤座', scorpio: '天蝎座',
    sagittarius: '射手座', capricorn: '摩羯座', aquarius: '水瓶座', pisces: '双鱼座'
  };
  var ageLabels = { under_18: '18 岁以下', '18_24': '18–24 岁', '25_34': '25–34 岁', '35_44': '35–44 岁', '45_54': '45–54 岁', '55_plus': '≥55 岁', unknown: '未填写 / 无效' };
  var trackOutputLabels = { zero: '0 条', one: '1 条', '2_5': '2–5 条', '6_20': '6–20 条', '21_plus': '≥21 条' };
  var pieColors = ['#206bc4', '#2fb344', '#f59f00', '#ae3ec9', '#d63939', '#17a2b8', '#6f42c1', '#74b816', '#f76707', '#0ca678', '#4263eb', '#e64980', '#868e96'];
  var provinceNameMap = {
    '北京市': '北京', '天津市': '天津', '上海市': '上海', '重庆市': '重庆',
    '内蒙古自治区': '内蒙古', '广西壮族自治区': '广西', '西藏自治区': '西藏',
    '宁夏回族自治区': '宁夏', '新疆维吾尔自治区': '新疆',
    '香港特别行政区': '香港', '澳门特别行政区': '澳门', '台湾省': '台湾'
  };
  var userCharts = Object.create(null);
  var userMap;
  var provinceGeoPromise;
  var provinceStats = Object.create(null);
  var provinceNames = Object.create(null);
  var selectedProvinceCode = '';
  function number(value) { return Number(value || 0); }
  function compactNumber(value) {
    var count = number(value);
    if (count < 10000) return count.toLocaleString();
    var scaled = count / 10000;
    return (scaled >= 10 ? scaled.toFixed(0) : scaled.toFixed(1)).replace(/\.0$/, '') + 'w';
  }
  function renderPie(chartID, legendID, items, total) {
    var chartRoot = document.getElementById(chartID), legendRoot = document.getElementById(legendID);
    if (!chartRoot || !legendRoot || !window.echarts) return;
    var rows = (items || []).map(function (item) { return { key: item.key, name: item.label || distributionLabels[item.key] || item.key || '未知', value: number(item.count) }; });
    var totalValue = number(total);
    if (!userCharts[chartID]) userCharts[chartID] = echarts.init(chartRoot);
    userCharts[chartID].setOption({
      color: pieColors,
      tooltip: { trigger: 'item', formatter: function (item) { return esc(item.name) + '<br>' + number(item.value).toLocaleString() + ' 人（' + item.percent + '%）'; } },
      series: [{ type: 'pie', radius: ['57%', '82%'], center: ['50%', '50%'], avoidLabelOverlap: true, itemStyle: { borderColor: '#fff', borderWidth: 2 }, label: { show: false }, emphasis: { scale: true, scaleSize: 6 }, data: rows }],
      graphic: [{ type: 'text', left: 'center', top: '41%', style: { text: totalValue.toLocaleString(), fill: '#182433', fontSize: 22, fontWeight: 700, textAlign: 'center' } }, { type: 'text', left: 'center', top: '55%', style: { text: chartID === 'userGenderChart' ? '全部用户' : '用户数', fill: '#667085', fontSize: 11, textAlign: 'center' } }]
    }, true);
    legendRoot.innerHTML = rows.map(function (item, index) {
      return '<div class="admin-user-legend-row"><span class="admin-user-legend-label"><i style="background:' + pieColors[index % pieColors.length] + '"></i>' + esc(item.name) + '</span><strong>' + item.value.toLocaleString() + '</strong></div>';
    }).join('') + (chartID === 'userGenderChart' ? '<div class="admin-user-legend-row admin-user-legend-total"><span>全部用户</span><strong>' + totalValue.toLocaleString() + '</strong></div>' : '');
  }
  function renderUserBars(chartID, items, labels, total) {
    var root = document.getElementById(chartID);
    if (!window.echarts) { root.textContent = '图表组件暂不可用'; return; }
    var rows = (items || []).map(function (item) {
      return { name: labels[item.key] || item.key, value: number(item.count), itemStyle: { color: item.key === 'unknown' || item.key === 'zero' ? '#94a3b8' : '#206bc4' } };
    });
    function percent(value) { return number(total) > 0 ? (value * 100 / number(total)).toFixed(1) : '0.0'; }
    if (!userCharts[chartID]) userCharts[chartID] = echarts.init(root);
    userCharts[chartID].setOption({
      aria: { enabled: true },
      grid: { left: 0, right: 82, top: 12, bottom: 8, containLabel: true },
      tooltip: { trigger: 'item', formatter: function (item) { return esc(item.name) + '<br>' + number(item.value).toLocaleString() + ' 人 · ' + percent(item.value) + '%'; } },
      xAxis: { type: 'value', show: false, max: function (value) { return Math.max(1, value.max); } },
      yAxis: { type: 'category', inverse: true, data: rows.map(function (item) { return item.name; }), axisLine: { show: false }, axisTick: { show: false }, axisLabel: { color: '#667085', fontSize: 11 } },
      series: [{ type: 'bar', data: rows, barMaxWidth: 12, showBackground: true, backgroundStyle: { color: '#f1f5f9', borderRadius: 4 }, itemStyle: { borderRadius: 4 }, label: { show: true, position: 'right', color: '#475569', fontSize: 10, formatter: function (item) { return compactNumber(item.value) + ' · ' + percent(item.value) + '%'; } } }]
    }, true);
  }
  function domesticProvinceCode(cityCode) {
    var code = String(cityCode || '');
    var prefixes = ['11','12','13','14','15','21','22','23','31','32','33','34','35','36','37','41','42','43','44','45','46','50','51','52','53','54','61','62','63','64','65','71','81','82'];
    return /^\d{6}$/.test(code) && prefixes.indexOf(code.slice(0, 2)) >= 0 ? code.slice(0, 2) + '0000' : '';
  }
  function chooseDefaultProvince() {
    if (selectedProvinceCode) return;
    var bestCode = '', bestTotal = -1;
    Object.keys(provinceStats).forEach(function (code) {
      if (provinceStats[code].total > bestTotal) { bestCode = code; bestTotal = provinceStats[code].total; }
    });
    selectedProvinceCode = bestCode;
  }
  function renderCityTop() {
    chooseDefaultProvince();
    var province = provinceStats[selectedProvinceCode];
    var provinceName = provinceNames[selectedProvinceCode] || selectedProvinceCode;
    document.getElementById('userCityStatsTitle').textContent = provinceName ? provinceName + ' · 城市用户数 Top 10' : '城市用户数 Top 10';
    var cities = province ? province.items.slice().sort(function (a, b) { return number(b.count) - number(a.count); }) : [];
    var topCities = cities.slice(0, 10), max = Math.max(1, ...topCities.map(function (item) { return number(item.count); }));
    document.getElementById('userCityStats').innerHTML = topCities.map(function (item) {
      var label = item.label || item.key || '未知';
      var count = number(item.count);
      return '<div class="admin-distribution-row"><span title="' + esc(label) + '">' + esc(label) + '</span><div class="admin-distribution-track"><i style="width:' + Math.round(count * 100 / max) + '%"></i></div><strong>' + count.toLocaleString() + '</strong></div>';
    }).join('') || '<span class="hint">该省暂无用户城市数据</span>';
  }
  function loadProvinceGeo() {
    if (!provinceGeoPromise) {
      provinceGeoPromise = fetch('/admin/static/china_provinces.json').then(function (response) {
        if (!response.ok) throw new Error('省份地图加载失败');
        return response.json();
      }).catch(function (error) { provinceGeoPromise = null; throw error; });
    }
    return provinceGeoPromise;
  }
  function renderProvinceMap() {
    var root = document.getElementById('userProvinceMap');
    if (!window.echarts) { root.textContent = '图表组件暂不可用'; return; }
    loadProvinceGeo().then(function (geo) {
      var features = (geo.features || []).filter(function (feature) {
        return String(feature.properties.adcode) !== '100000_JD';
      });
      echarts.registerMap('china_provinces', { type: 'FeatureCollection', features: features });
      var nameMap = {};
      features.forEach(function (feature) {
        var fullName = feature.properties.name;
        var code = String(feature.properties.adcode);
        provinceNames[code] = fullName;
        nameMap[fullName] = provinceNameMap[fullName] || fullName.replace(/[省市]$/, '');
      });
      chooseDefaultProvince();
      renderCityTop();
      var values = Object.keys(provinceStats).map(function (code) { return provinceStats[code].total; });
      var max = Math.max(1, ...values);
      var data = features.map(function (feature) {
        var code = String(feature.properties.adcode);
        var fullName = feature.properties.name;
        return {
          name: nameMap[fullName], fullName: fullName, adcode: code,
          value: provinceStats[code] ? provinceStats[code].total : 0,
          selected: code === selectedProvinceCode
        };
      });
      if (!userMap) userMap = echarts.init(root);
      userMap.setOption({
        tooltip: { trigger: 'item', formatter: function (params) {
          if (!params.data || !params.data.fullName) return '';
          return esc(params.data.fullName) + '<br>' + number(params.data.value).toLocaleString() + ' 位用户';
        } },
        visualMap: { show: false, seriesIndex: 0, min: 0, max: max, inRange: { color: ['#edf3fa', '#8db9e8', '#206bc4'] } },
        series: [{
          type: 'map', map: 'china_provinces', nameMap: nameMap, selectedMode: 'single', roam: false,
          boundingCoords: [[73, 54], [136, 17]], layoutCenter: ['50%', '50%'], layoutSize: '105%',
          label: { show: true, color: '#334155', fontSize: 9, formatter: function (params) {
            return params.data && number(params.data.value) > 0 ? compactNumber(params.data.value) : '';
          } },
          emphasis: { label: { show: true, color: '#102a43', fontWeight: 700 }, itemStyle: { areaColor: '#f2b84b' } },
          select: { label: { show: true, color: '#fff', fontWeight: 700 }, itemStyle: { areaColor: '#174f94' } },
          itemStyle: { borderColor: '#fff', borderWidth: 1, areaColor: '#edf3fa' },
          data: data
        }]
      }, true);
      userMap.off('click');
      userMap.on('click', function (params) {
        if (!params.data || !params.data.adcode) return;
        selectedProvinceCode = String(params.data.adcode);
        renderCityTop();
        renderProvinceMap();
      });
    }).catch(function () { root.textContent = '省份地图暂不可用，请查看下方城市统计'; });
  }
  function renderProvinceStats(items) {
    provinceStats = Object.create(null);
    var foreignTotal = 0, foreignCities = 0, unknownTotal = 0;
    (items || []).forEach(function (item) {
      var key = String(item.key || ''), count = number(item.count), provinceCode = domesticProvinceCode(key);
      if (key === 'unknown' || !key) { unknownTotal += count; return; }
      if (!provinceCode) { foreignTotal += count; foreignCities++; return; }
      if (!provinceStats[provinceCode]) provinceStats[provinceCode] = { total: 0, items: [] };
      provinceStats[provinceCode].total += count;
      provinceStats[provinceCode].items.push(item);
    });
    document.getElementById('userForeignCityTotal').textContent = foreignTotal.toLocaleString();
    document.getElementById('userForeignCityNote').textContent = foreignCities ? foreignCities + ' 个海外城市；每个用户只计入当前所在城市' : '当前没有海外城市数据';
    document.getElementById('userUnknownCityTotal').textContent = unknownTotal.toLocaleString();
    if (!provinceStats[selectedProvinceCode]) selectedProvinceCode = '';
    renderCityTop();
    renderProvinceMap();
  }
  async function loadCities() {
    try {
      var response = await fetch('/admin/api/cities');
      if (!response.ok) throw new Error(String(response.status));
      var cities = (await response.json()).data || [];
      cities.forEach(function (city) {
        var option = document.createElement('option');
        option.value = city.code;
        option.textContent = city.name;
        cityFilter.appendChild(option);
      });
    } catch (error) {
      AdminUI.alert('城市列表加载失败: ' + (error.message || error), 'danger');
    }
  }
  var statisticsLoading = false, statisticsLoaded = false;
  var statisticsRefreshBtn = document.getElementById('refreshUserStatistics');
  statisticsRefreshBtn.addEventListener('click', loadUserStatistics);
  async function loadUserStatistics() {
    if (statisticsLoading) return;
    statisticsLoading = true;
    statisticsRefreshBtn.disabled = true;
    statisticsRefreshBtn.textContent = '刷新中…';
    document.getElementById('userStatistics').setAttribute('aria-busy', 'true');
    var errorNode = document.getElementById('userStatisticsError');
    errorNode.hidden = true;
    try {
      var response = await fetch('/admin/api/users/statistics', { cache: 'no-store' });
      if (!response.ok) throw new Error('统计加载失败');
      var data = (await response.json()).data || {};
      renderPie('userGenderChart', 'userGenderStats', data.gender, data.total_users);
      renderPie('userZodiacChart', 'userZodiacStats', data.zodiac, data.total_users);
      renderProvinceStats(data.user_cities || []);
      renderUserBars('userAgeChart', data.age, ageLabels, data.total_users);
      renderUserBars('userTrackOutputChart', data.track_output, trackOutputLabels, data.total_users);
      statisticsLoaded = true;
    } catch (_) {
      errorNode.textContent = statisticsLoaded ? '刷新失败，当前仍显示上次数据，请重试。' : '统计加载失败，请点击“刷新数据”重试。';
      errorNode.hidden = false;
    } finally {
      statisticsLoading = false;
      statisticsRefreshBtn.disabled = false;
      statisticsRefreshBtn.textContent = '刷新数据';
      document.getElementById('userStatistics').setAttribute('aria-busy', 'false');
    }
  }

  // 翻页：采用 cursor 栈记录每一页的入口 cursor。
  // cursorStack[i] = 第 i 页（从 0 开始）的入口 cursor；首页入口 cursor 为 null。
  var pageSize = 20;
  var cursorStack = [null];
  var pageIndex = 0;
  var lastNextCursor = null;
  var totalCount = 0;

  var tbody = document.getElementById('userList');
  var prevBtn = document.getElementById('prevBtn');
  var nextBtn = document.getElementById('nextBtn');
  var refreshBtn = document.getElementById('refreshBtn');
  var searchBtn = document.getElementById('searchBtn');
  var titleFilter = document.getElementById('titleFilter');
  var internalFilter = document.getElementById('internalFilter');
  var cityFilter = document.getElementById('cityFilter');
  var genderFilter = document.getElementById('genderFilter');
  var zodiacFilter = document.getElementById('zodiacFilter');
  var restrictedFilter = document.getElementById('restrictedFilter');
  var pageHint = document.getElementById('pageHint');
  var totalHint = document.getElementById('totalHint');

  var detailModal = new tabler.Modal(document.getElementById('userDetailModal'), { backdrop: 'static' });
  var detailForm = document.getElementById('userDetailForm');
  var detailSave = document.getElementById('userDetailSave');
  var activeDetailUserID = 0;

  function locationText(user) {
    var province = String(user.province_name || '').trim();
    var city = String(user.city_name || '').trim();
    if (province && city && province !== city) return province + ' · ' + city;
    return city || province || '-';
  }

  async function openUserDetail(userID) {
    activeDetailUserID = userID;
    detailSave.disabled = true;
    document.getElementById('userDetailName').textContent = '加载中…';
    document.getElementById('userDetailMeta').textContent = '';
    document.getElementById('userDetailAvatar').removeAttribute('src');
    detailModal.show();
    try {
      var response = await fetch('/admin/api/users/' + encodeURIComponent(userID));
      if (!response.ok) throw new Error(await response.text());
      var user = (await response.json()).data || {};
      if (activeDetailUserID !== userID) return;
      detailForm.elements.nickname.value = user.nickname || '';
      detailForm.elements.phone.value = user.phone || '';
      detailForm.elements.gender.value = user.gender || '';
      detailForm.elements.birthday.value = user.birthday || '';
      detailForm.elements.birthday.max = new Date().toISOString().slice(0, 10);
      detailForm.elements.signature.value = user.signature || '';
      detailForm.elements.is_internal.checked = !!user.is_internal;
      document.getElementById('userDetailLocation').value = locationText(user);
      document.getElementById('userDetailName').textContent = user.nickname || '未设置昵称';
      document.getElementById('userDetailMeta').textContent = '注册：' + fmtTime(user.created_at) + '　更新：' + fmtTime(user.updated_at);
      if (user.avatar_url) document.getElementById('userDetailAvatar').src = user.avatar_url;
      detailSave.disabled = false;
    } catch (error) {
      if (activeDetailUserID !== userID) return;
      detailModal.hide();
      AdminUI.alert('读取用户资料失败: ' + error.message, 'danger');
    }
  }

  detailForm.addEventListener('submit', async function (event) {
    event.preventDefault();
    if (!activeDetailUserID || !detailForm.reportValidity()) return;
    detailSave.disabled = true;
    var payload = {
      nickname: detailForm.elements.nickname.value.trim(),
      phone: detailForm.elements.phone.value.trim(),
      gender: detailForm.elements.gender.value,
      birthday: detailForm.elements.birthday.value,
      signature: detailForm.elements.signature.value,
      is_internal: detailForm.elements.is_internal.checked
    };
    try {
      var response = await fetch('/admin/api/users/' + encodeURIComponent(activeDetailUserID), {
        method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload)
      });
      if (!response.ok) throw new Error(await response.text());
      detailModal.hide();
      AdminUI.alert('用户资料已保存', 'success');
      loadPage();
    } catch (error) {
      AdminUI.alert('保存失败: ' + error.message, 'danger');
      detailSave.disabled = false;
    }
  });

  tbody.addEventListener('click', function (ev) {
    var btn = ev.target.closest('[data-action="detail"]');
    if (!btn) return;
    openUserDetail(btn.dataset.userId);
  });

  tbody.addEventListener('change', async function (ev) {
    var input = ev.target;
    if (!input.dataset || input.dataset.action !== 'restriction') return;
    var userID = input.dataset.userId;
    var restrict = input.checked;
    input.disabled = true;
    if (restrict) {
      var values = await AdminUI.form({
        title: '限制用户 #' + userID,
        confirmText: '确认限制',
        fields: [
          { name: 'reason', label: '限制原因', value: '违规上传内容', required: true, maxLength: 500 },
          { name: 'days', label: '限制天数', value: '7', type: 'number', min: 1, placeholder: '留空表示永久' }
        ]
      });
      if (!values) { input.checked = false; input.disabled = false; return; }
      var daysRaw = values.days.trim();
      var payload = { reason: values.reason.trim() };
      if (daysRaw) {
        var days = Number(daysRaw);
        if (!isFinite(days) || days <= 0) {
          AdminUI.alert('限制天数必须大于 0', 'danger');
          input.checked = false; input.disabled = false; return;
        }
        payload.expires_at = new Date(Date.now() + days * 24 * 60 * 60 * 1000).toISOString();
      }
      var response = await fetch('/admin/api/users/' + encodeURIComponent(userID) + '/restrictions', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload)
      });
      if (!response.ok) {
        AdminUI.alert('限制失败: ' + await response.text(), 'danger');
        input.checked = false; input.disabled = false; return;
      }
      AdminUI.alert('账号限制已生效', 'success');
      loadPage();
      return;
    }
    var confirmed = await AdminUI.confirm({ title: '解除账号限制', message: '确定解除用户 #' + userID + ' 当前的账号限制？', confirmText: '解除限制' });
    if (!confirmed) { input.checked = true; input.disabled = false; return; }
    var revokeResponse = await fetch('/admin/api/users/' + encodeURIComponent(userID) + '/restrictions/current', { method: 'DELETE' });
    if (!revokeResponse.ok) {
      AdminUI.alert('解除失败: ' + await revokeResponse.text(), 'danger');
      input.checked = true; input.disabled = false; return;
    }
    AdminUI.alert('账号限制已解除', 'success');
    loadPage();
  });

  refreshBtn.addEventListener('click', function () {
    cursorStack = [null];
    pageIndex = 0;
    loadPage();
  });
  function resetSearch() { cursorStack = [null]; pageIndex = 0; loadPage(); }
  searchBtn.addEventListener('click', resetSearch);
  titleFilter.addEventListener('keydown', function (event) { if (event.key === 'Enter') resetSearch(); });
  [internalFilter, cityFilter, genderFilter, zodiacFilter, restrictedFilter].forEach(function (filter) {
    filter.addEventListener('change', resetSearch);
  });
  prevBtn.addEventListener('click', function () {
    if (pageIndex <= 0) return;
    pageIndex -= 1;
    cursorStack = cursorStack.slice(0, pageIndex + 1);
    loadPage();
  });
  nextBtn.addEventListener('click', function () {
    if (!lastNextCursor) return;
    pageIndex += 1;
    cursorStack[pageIndex] = lastNextCursor;
    loadPage();
  });

  async function loadPage() {
    var params = new URLSearchParams();
    params.set('limit', String(pageSize));
    if (titleFilter.value.trim()) params.set('title', titleFilter.value.trim());
    if (internalFilter.value) params.set('is_internal', internalFilter.value);
    if (cityFilter.value) params.set('city_code', cityFilter.value);
    if (genderFilter.value) params.set('gender', genderFilter.value);
    if (zodiacFilter.value) params.set('zodiac_sign', zodiacFilter.value);
    if (restrictedFilter.value) params.set('restricted', restrictedFilter.value);
    var cur = cursorStack[pageIndex];
    if (cur) {
      params.set('cursor_created_at', cur.created_at);
      params.set('cursor_id', String(cur.id));
    }
    var resp = await fetch('/admin/api/users?' + params.toString());
    if (!resp.ok) {
      tbody.innerHTML = '<tr><td colspan="8">加载失败: ' + resp.status + '</td></tr>';
      return;
    }
    var data = ((await resp.json()).data) || {};
    var items = data.items || [];
    totalCount = data.total || 0;
    lastNextCursor = data.next_cursor || null;

    tbody.innerHTML = '';
    items.forEach(function (it) {
      var avatar = it.avatar_url
        ? '<img class="admin-user-avatar" src="' + esc(it.avatar_url) + '" alt="" />'
        : '<span class="admin-user-avatar admin-user-avatar-empty" aria-hidden="true">👤</span>';
      avatar = '<span class="admin-user-avatar-wrap">' + avatar + (it.is_internal ? '<span class="admin-user-inner-badge">inner</span>' : '') + '</span>';
      var tr = document.createElement('tr');
      tr.innerHTML = '<td><div class="admin-user-identity">' + avatar + '<span>' + esc(it.nickname) + '</span></div></td>' +
        '<td>' + esc(it.phone || '') + '</td>' +
        '<td>' + esc(genderText(it.gender)) + '</td>' +
        '<td>' + esc(zodiacText(it.zodiac_sign)) + '</td>' +
        '<td><span class="admin-user-city-cell">' + esc(locationText(it)) + '</span></td>' +
        '<td>' + restrictionText(it.account_restriction) + '</td>' +
        '<td>' + (it.last_login_at ? fmtTime(it.last_login_at) : '—') + '</td>' +
        '<td><div class="admin-user-actions"><button data-action="detail" data-user-id="' + esc(it.id) + '">查看详情</button>' +
        '<label class="form-switch admin-user-restriction-switch"><input type="checkbox" data-action="restriction" data-user-id="' + esc(it.id) + '"' + (it.account_restriction ? ' checked' : '') + '><span>限制</span></label></div></td>';
      tbody.appendChild(tr);
    });

    prevBtn.disabled = pageIndex <= 0;
    nextBtn.disabled = !lastNextCursor;
    pageHint.textContent = '第 ' + (pageIndex + 1) + ' 页（' + items.length + ' 条）';
    totalHint.textContent = '共 ' + totalCount + ' 个用户';
  }

  loadUserStatistics();
  loadCities();
  loadPage();
  window.addEventListener('resize', function () {
    Object.keys(userCharts).forEach(function (id) { userCharts[id].resize(); });
    if (userMap) userMap.resize();
  });
})();
