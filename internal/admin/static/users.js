(function () {
  // 通用：登录态校验 + 退出

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  function fmtTime(value) {
    return esc(AdminTime.dateTime(value));
  }
  function activeDaysText(value) {
    return Number.isInteger(value) && value >= 0 && value <= 7 ? value + ' 天' : '—';
  }
  function activeAppVersionText(value) {
    return typeof value === 'string' && value.trim() ? 'App ' + value.trim() : '';
  }
  function genderText(value) {
    return value === 'male' ? '男' : (value === 'female' ? '女' : '-');
  }


  var distributionLabels = {
    male: '男', female: '女', unknown: '未知', aries: '白羊座', taurus: '金牛座', gemini: '双子座',
    cancer: '巨蟹座', leo: '狮子座', virgo: '处女座', libra: '天秤座', scorpio: '天蝎座',
    sagittarius: '射手座', capricorn: '摩羯座', aquarius: '水瓶座', pisces: '双鱼座'
  };
  var ageLabels = { under_18: '18 岁以下', '18_24': '18–24 岁', '25_34': '25–34 岁', '35_44': '35–44 岁', '45_54': '45–54 岁', '55_plus': '≥55 岁', unknown: '未填写 / 无效' };
  var trackOutputLabels = { zero: '0 条', one: '1 条', '2_5': '2–5 条', '6_20': '6–20 条', '21_plus': '≥21 条' };
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
  function renderPie(chartID, legendID, items, total, labels) {
    var chartRoot = document.getElementById(chartID), legendRoot = document.getElementById(legendID);
    if (!chartRoot || !legendRoot) return;
    labels = labels || distributionLabels;
    var rows = (items || []).map(function (item) { return { key: item.key, name: item.label || labels[item.key] || item.key || '未知', value: number(item.count) }; });
    var totalValue = number(total);
    AdminChartStyle.legend(legendRoot, rows, chartID === 'userGenderChart' ? '全部用户' : '', totalValue);
    if (!window.echarts) return;
    if (!userCharts[chartID]) userCharts[chartID] = echarts.init(chartRoot);
    userCharts[chartID].setOption(AdminChartStyle.ring(rows, totalValue, chartID === 'userGenderChart' ? '全部用户' : '用户数', '人'), true);
  }
  function renderUserBars(chartID, items, labels, total) {
    var root = document.getElementById(chartID);
    if (!window.echarts) { root.textContent = '图表组件暂不可用'; return; }
    var rows = (items || []).map(function (item) {
      return { name: labels[item.key] || item.key, value: number(item.count), itemStyle: { color: item.key === 'unknown' || item.key === 'zero' ? '#94a3b8' : '#206bc4' } };
    });
    if (!userCharts[chartID]) userCharts[chartID] = echarts.init(root);
    userCharts[chartID].setOption(AdminChartStyle.bars(rows, total, { unit: '人', showPercent: true }), true);
  }
  function renderLocationTop() {
    var province = provinceStats[selectedProvinceCode];
    var provinceName = provinceNames[selectedProvinceCode] || selectedProvinceCode;
    document.getElementById('userCityStatsTitle').textContent = selectedProvinceCode ? provinceName + ' · 城市用户数 Top 10' : '省份用户数 Top 10';
    var rows = selectedProvinceCode ? (province ? province.items.slice() : []) : Object.keys(provinceStats).map(function (code) {
      return { name: provinceNames[code] || code, count: provinceStats[code].total };
    });
    rows.sort(function (a, b) { return number(b.count) - number(a.count); });
    var topRows = rows.slice(0, 10), max = Math.max(1, ...topRows.map(function (item) { return number(item.count); }));
    document.getElementById('userCityStats').innerHTML = topRows.map(function (item) {
      var label = item.name || '未知';
      var count = number(item.count);
      return '<div class="admin-distribution-row"><span title="' + esc(label) + '">' + esc(label) + '</span><div class="admin-distribution-track"><i style="width:' + Math.round(count * 100 / max) + '%"></i></div><strong>' + count.toLocaleString() + '</strong></div>';
    }).join('') || '<span class="hint">' + (selectedProvinceCode ? '该省暂无用户城市数据' : '暂无省份用户数据') + '</span>';
  }
  function loadProvinceGeo() {
    if (!provinceGeoPromise) {
      provinceGeoPromise = fetch(AdminAssets.url('/admin/static/china_provinces.json')).then(function (response) {
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
      renderLocationTop();
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
      if (!userMap) {
        userMap = echarts.init(root);
        userMap.getZr().on('click', function (event) {
          if (event.target || !selectedProvinceCode) return;
          selectedProvinceCode = '';
          renderLocationTop();
          renderProvinceMap();
        });
      }
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
        renderLocationTop();
        renderProvinceMap();
      });
    }).catch(function () { root.textContent = '省份地图暂不可用，请查看下方位置统计'; });
  }
  function renderProvinceStats(items, drawMap) {
    provinceStats = Object.create(null);
    var foreignTotal = 0, foreignCities = 0, unknownTotal = 0;
    (items || []).forEach(function (item) {
      var count = number(item.count), countryCode = item.country_code, provinceCode = item.province_map_code;
      if (!item.region_id || !countryCode) { unknownTotal += count; return; }
      if (countryCode !== 'CN') { foreignTotal += count; foreignCities++; return; }
      if (!provinceCode) { unknownTotal += count; return; }
      provinceNames[provinceCode] = item.province_name;
      if (!provinceStats[provinceCode]) provinceStats[provinceCode] = { total: 0, items: [] };
      provinceStats[provinceCode].total += count;
      provinceStats[provinceCode].items.push(item);
    });
    document.getElementById('userForeignCityTotal').textContent = foreignTotal.toLocaleString();
    document.getElementById('userForeignCityNote').textContent = foreignCities ? foreignCities + ' 个海外城市；每个用户只计入当前所在城市' : '当前没有海外城市数据';
    document.getElementById('userUnknownCityTotal').textContent = unknownTotal.toLocaleString();
    if (!provinceStats[selectedProvinceCode]) selectedProvinceCode = '';
    renderLocationTop();
    if (drawMap !== false) renderProvinceMap();
  }
  var filterRegions = [];
  function populateFilterCities() {
    cityFilter.replaceChildren(new Option('全部城市', ''));
    filterRegions.filter(function (item) { return (!provinceFilter.value || item.province_code === provinceFilter.value || item.code === provinceFilter.value); }).forEach(function (item) { cityFilter.add(new Option(item.name, item.code)); });
  }
  async function loadCities() {
    try {
      var response = await fetch('/admin/api/cities');
      if (!response.ok) throw new Error(String(response.status));
      var cities = (await response.json()).data || [];
      filterRegions = cities;
      cities.filter(function (item) { return item.level === 'province'; }).forEach(function (item) { provinceFilter.add(new Option(item.name, item.code)); });
      populateFilterCities();
      AdminRegionPicker.enhance(provinceFilter, cityFilter, cities);
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
      renderPie('userAgeChart', 'userAgeStats', data.age, data.total_users, ageLabels);
      renderProvinceStats(data.user_cities || [], false);
      await AdminAssets.loadCharts();
      renderPie('userGenderChart', 'userGenderStats', data.gender, data.total_users);
      renderPie('userZodiacChart', 'userZodiacStats', data.zodiac, data.total_users);
      renderProvinceMap();
      renderPie('userAgeChart', 'userAgeStats', data.age, data.total_users, ageLabels);
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
  var listRequest = 0;

  var tbody = document.getElementById('userList');
  var prevBtn = document.getElementById('prevBtn');
  var nextBtn = document.getElementById('nextBtn');
  var refreshBtn = document.getElementById('refreshBtn');
  var searchBtn = document.getElementById('searchBtn');
  var titleFilter = document.getElementById('titleFilter');
  var internalFilter = document.getElementById('internalFilter');
  var provinceFilter = document.getElementById('provinceFilter');
  var cityFilter = document.getElementById('cityFilter');
  var genderFilter = document.getElementById('genderFilter');
  var levelFilter = document.getElementById('levelFilter');
  var restrictedFilter = document.getElementById('restrictedFilter');
  var pageHint = document.getElementById('pageHint');
  var totalHint = document.getElementById('totalHint');

  var detailRoot = document.getElementById('userDetailModal');
  var detailModal = new tabler.Modal(detailRoot, { backdrop: 'static' });
  var detailForm = document.getElementById('userDetailForm'), detailSave = document.getElementById('userDetailSave');
  var profileFields = document.getElementById('userProfileFields');
  var activeDetailUserID = '', userDetailRequest = 0, userDetailController = null;
  var profileReady = false, profileSaving = false, overviewLoading = false, permissionLoadedFor = '';
  var restriction = null, restrictionReady = false, restrictionBusy = false, restrictionMode = '';
  var backgroundURL = '', backgroundTimer = null;
  var background = document.getElementById('userDetailBackground');
  var backgroundHint = document.getElementById('userBackgroundPlaceholder');
  var backgroundRetry = document.getElementById('userBackgroundRetry');
  var restrictionAction = document.getElementById('userRestrictionAction');
  var restrictionConfirm = document.getElementById('userRestrictionConfirm');
  var restrictionEditor = document.getElementById('userRestrictionEditor');
  var restrictionError = document.getElementById('userRestrictionError');
  function currentDetail(userID, request) { return userID === activeDetailUserID && request === userDetailRequest; }
  detailRoot.addEventListener('hide.bs.modal', function () {
    userDetailRequest++;
    if (userDetailController) userDetailController.abort();
    AdminUserPermissions.cancel();
    cancelBackground(); backgroundURL = '';
  });
  detailRoot.addEventListener('hidden.bs.modal', function () { activeDetailUserID = ''; profileReady = false; });
  function locationText(user) {
    var province = String(user.province_name || '').trim(), city = String(user.city_name || '').trim();
    return province && city && province !== city ? province + ' · ' + city : city || province || '-';
  }
  async function userJSON(path, options) {
    var response = await fetch('/admin/api/users/' + path, options);
    var payload = await response.json().catch(function () { return {}; });
    if (!response.ok) throw new Error(payload.error || '请求失败：' + response.status);
    return payload.data;
  }
  function renderBirthday(serverCode) {
    AdminUserZodiac.render(document.getElementById('userDetailZodiac'), detailForm.elements.birthday.value, serverCode);
  }
  detailForm.elements.birthday.addEventListener('input', function () { renderBirthday(); });
  detailForm.elements.birthday.addEventListener('change', function () { renderBirthday(); });
  function cancelBackground() {
    clearTimeout(backgroundTimer); backgroundTimer = null;
    background.onload = null; background.onerror = null;
    background.removeAttribute('src');
  }
  function loadBackground() {
    cancelBackground();
    background.hidden = true; backgroundHint.hidden = false; backgroundRetry.hidden = true;
    backgroundHint.textContent = backgroundURL ? '正在加载背景图…' : '未设置背景图';
    if (!backgroundURL) return;
    var userID = activeDetailUserID, request = userDetailRequest, settled = false;
    // Each attempt owns its element, so a canceled request cannot finish a later user's load.
    var image = document.createElement('img');
    image.id = 'userDetailBackground'; image.alt = '用户背景图'; image.hidden = true;
    background.replaceWith(image); background = image;
    function finish(success, timedOut) {
      if (settled || background !== image || !currentDetail(userID, request)) return;
      settled = true;
      clearTimeout(backgroundTimer); backgroundTimer = null;
      image.onload = null; image.onerror = null;
      image.hidden = !success; backgroundHint.hidden = success; backgroundRetry.hidden = success;
      if (!success) {
        image.removeAttribute('src');
        backgroundHint.textContent = timedOut ? '背景图加载超时，请重试' : '背景图加载失败';
      }
    }
    image.onload = function () { finish(image.naturalWidth > 0, false); };
    image.onerror = function () { finish(false, false); };
    backgroundTimer = setTimeout(function () { finish(false, true); }, 30000);
    image.src = backgroundURL;
  }
  backgroundRetry.addEventListener('click', loadBackground);
  document.getElementById('userDetailAvatar').addEventListener('load', function () {
    if (!activeDetailUserID || userDetailController.signal.aborted || !this.getAttribute('src')) return;
    this.hidden = false; document.getElementById('userDetailAvatarPlaceholder').hidden = true;
  });
  document.getElementById('userDetailAvatar').addEventListener('error', function () {
    this.hidden = true; this.removeAttribute('src'); document.getElementById('userDetailAvatarPlaceholder').hidden = false;
  });
  async function loadOverview(userID, request, signal) {
    if (overviewLoading) return;
    overviewLoading = true; document.getElementById('userOverviewError').hidden = true;
    try {
      var overview = await userJSON(encodeURIComponent(userID) + '/overview', { signal: signal });
      if (!currentDetail(userID, request)) return;
      [['userFollowingCount','following_count'],['userFollowerCount','follower_count'],['userReceivedLikeCount','received_like_count']].forEach(function (field) {
        document.getElementById(field[0]).textContent = Number(overview[field[1]]).toLocaleString();
      });
      var level = document.getElementById('userDetailLevel');
      level.textContent = 'Lv.' + overview.level.level + ' · ' + overview.level.name; level.hidden = false;
    } catch (error) {
      if (currentDetail(userID, request) && !signal.aborted) document.getElementById('userOverviewError').hidden = false;
    } finally { if (currentDetail(userID, request)) overviewLoading = false; }
  }
  document.getElementById('userOverviewRetry').addEventListener('click', function () {
    if (activeDetailUserID && userDetailController) loadOverview(activeDetailUserID, userDetailRequest, userDetailController.signal);
  });
  function renderRestriction() {
    var badge = document.getElementById('userRestrictionStatus'), info = document.getElementById('userRestrictionInfo');
    badge.textContent = restrictionReady ? (restriction ? '已限制' : '正常') : '未知';
    badge.className = 'badge ' + (restrictionReady ? (restriction ? 'bg-red-lt' : 'bg-green-lt') : 'bg-secondary-lt');
    info.textContent = restrictionReady ? (restriction ? (restriction.reason || '未填写原因') + '\n' + (restriction.expires_at ? '至 ' + fmtTime(restriction.expires_at) : '永久限制') + (restriction.operator ? '\n操作人：' + restriction.operator : '') : '') : '暂时无法读取限制状态。';
    restrictionAction.textContent = restriction ? '解除限制' : '限制账号';
    restrictionAction.classList.toggle('btn-outline-secondary', !!restriction);
    restrictionAction.classList.toggle('btn-outline-danger', !restriction);
    restrictionAction.disabled = !profileReady || !restrictionReady || restrictionBusy || profileSaving || !AdminAccess.can('users', true);
    detailForm.elements.is_internal.disabled = !profileReady || profileSaving || !AdminAccess.can('users', true);
    restrictionConfirm.disabled = restrictionBusy || profileSaving;
    document.getElementById('userRestrictionCancel').disabled = restrictionBusy;
    document.getElementById('userRestrictionReason').disabled = restrictionBusy || !AdminAccess.can('users', true);
    document.getElementById('userRestrictionDays').disabled = restrictionBusy || !AdminAccess.can('users', true);
  }
  async function loadRestriction(userID, request, signal) {
    document.getElementById('userRestrictionRetry').hidden = true;
    try {
      var response = await fetch('/admin/api/users/' + encodeURIComponent(userID) + '/restrictions/current', { signal: signal });
      var payload = await response.json().catch(function () { return {}; });
      if (response.status !== 404 && !response.ok) throw new Error(payload.error || '读取失败');
      if (!currentDetail(userID, request)) return;
      restriction = response.status === 404 ? null : payload.data; restrictionReady = true;
    } catch (error) {
      if (!currentDetail(userID, request) || signal.aborted) return;
      restrictionReady = false; document.getElementById('userRestrictionRetry').hidden = false;
    }
    if (currentDetail(userID, request)) renderRestriction();
  }
  document.getElementById('userRestrictionRetry').addEventListener('click', function () {
    if (activeDetailUserID && userDetailController) loadRestriction(activeDetailUserID, userDetailRequest, userDetailController.signal);
  });
  restrictionAction.addEventListener('click', function () {
    if (!restrictionReady || !profileReady || restrictionBusy || profileSaving || !AdminAccess.can('users', true)) return;
    restrictionMode = restriction ? 'revoke' : 'create'; restrictionEditor.hidden = false; restrictionAction.hidden = true; restrictionError.textContent = '';
    document.getElementById('userRestrictionCreateFields').hidden = restrictionMode !== 'create';
    document.getElementById('userRestrictionConfirmText').hidden = restrictionMode !== 'revoke';
    document.getElementById('userRestrictionReason').value = '违规上传内容'; document.getElementById('userRestrictionDays').value = '7';
    restrictionConfirm.textContent = restrictionMode === 'create' ? '确认限制' : '确认解除';
    restrictionConfirm.classList.toggle('btn-danger', restrictionMode === 'create');
    restrictionConfirm.classList.toggle('btn-primary', restrictionMode !== 'create');
  });
  document.getElementById('userRestrictionCancel').addEventListener('click', function () {
    if (restrictionBusy) return;
    restrictionEditor.hidden = true; restrictionAction.hidden = false; restrictionMode = ''; restrictionError.textContent = '';
  });
  restrictionConfirm.addEventListener('click', async function () {
    if (!restrictionMode || !profileReady || restrictionBusy || profileSaving || !AdminAccess.can('users', true)) return;
    var userID = activeDetailUserID, request = userDetailRequest, payload;
    var signal = userDetailController.signal, create = restrictionMode === 'create';
    if (create) {
      var reason = document.getElementById('userRestrictionReason').value.trim();
      var daysRaw = document.getElementById('userRestrictionDays').value.trim(), days = Number(daysRaw);
      if (!reason || Array.from(reason).length > 500) { restrictionError.textContent = '请填写 1–500 字的限制原因'; return; }
      if (daysRaw && (!Number.isSafeInteger(days) || days <= 0 || !Number.isFinite(Date.now() + days * 86400000) || Date.now() + days * 86400000 > 8640000000000000)) { restrictionError.textContent = '限制天数须为大于 0 的有效整数'; return; }
      payload = { reason: reason };
      if (daysRaw) payload.expires_at = new Date(Date.now() + days * 86400000).toISOString();
    }
    restrictionBusy = true; restrictionError.textContent = ''; detailSave.disabled = true; renderRestriction();
    try {
      await userJSON(encodeURIComponent(userID) + (create ? '/restrictions' : '/restrictions/current'), {
        method: create ? 'POST' : 'DELETE', signal: signal,
        headers: create ? { 'Content-Type': 'application/json' } : {}, body: create ? JSON.stringify(payload) : undefined
      });
      if (!currentDetail(userID, request)) return;
      restrictionEditor.hidden = true; restrictionAction.hidden = false; restrictionMode = '';
      AdminUI.alert(create ? '账号限制已生效' : '账号限制已解除', 'success');
      await loadRestriction(userID, request, signal);
    } catch (error) {
      if (currentDetail(userID, request) && !signal.aborted) restrictionError.textContent = error.message;
    } finally {
      if (currentDetail(userID, request)) { restrictionBusy = false; detailSave.disabled = !profileReady; renderRestriction(); }
    }
  });
  document.getElementById('userPermissionSection').addEventListener('toggle', function () {
    if (this.open && activeDetailUserID && permissionLoadedFor !== activeDetailUserID) {
      permissionLoadedFor = activeDetailUserID; AdminUserPermissions.load(activeDetailUserID);
    }
  });
  function openUserDetail(userID) {
    return AdminUI.runOnce('user-detail', function () { return loadUserDetail(userID); });
  }
  async function loadUserDetail(userID) {
    if (activeDetailUserID === userID && detailRoot.classList.contains('show')) return;
    var request = ++userDetailRequest;
    if (userDetailController) userDetailController.abort();
    var controller = userDetailController = new AbortController();
    activeDetailUserID = userID; profileReady = false; profileSaving = false; overviewLoading = false; permissionLoadedFor = '';
    restriction = null; restrictionReady = false; restrictionBusy = false; restrictionMode = '';
    detailForm.reset(); profileFields.disabled = true; detailSave.disabled = true; detailSave.textContent = '保存资料';
    document.getElementById('userDetailName').textContent = '加载中…';
    document.getElementById('userDetailMeta').textContent = ''; document.getElementById('userDetailID').textContent = 'ID ' + userID;
    document.getElementById('userDetailAvatar').removeAttribute('src'); document.getElementById('userDetailLevel').hidden = true;
    document.getElementById('userDetailAvatar').hidden = true; document.getElementById('userDetailAvatarPlaceholder').hidden = false;
    ['userFollowingCount','userFollowerCount','userReceivedLikeCount'].forEach(function (id) { document.getElementById(id).textContent = '—'; });
    document.getElementById('userPermissionSection').open = false; document.getElementById('userPermissionSnapshots').replaceChildren();
    restrictionEditor.hidden = true; restrictionAction.hidden = false; restrictionError.textContent = '';
    renderRestriction(); document.getElementById('userRestrictionStatus').textContent = '读取中';
    backgroundURL = ''; loadBackground(); renderBirthday(); detailModal.show();
    loadOverview(userID, request, controller.signal); loadRestriction(userID, request, controller.signal);
    try {
      var user = await userJSON(encodeURIComponent(userID), { signal: controller.signal });
      if (!currentDetail(userID, request)) return;
      detailForm.elements.nickname.value = user.nickname || ''; detailForm.elements.phone.value = user.phone || '';
      detailForm.elements.gender.value = user.gender || ''; detailForm.elements.birthday.value = user.birthday || '';
      detailForm.elements.birthday.max = AdminUserZodiac.today(); detailForm.elements.signature.value = user.signature || '';
      detailForm.elements.is_internal.checked = !!user.is_internal;
      document.getElementById('userDetailLocation').value = locationText(user);
      document.getElementById('userDetailName').textContent = user.nickname || '未设置昵称';
      var activeVersion = activeAppVersionText(user.last_active_app_version);
      document.getElementById('userDetailMeta').textContent = '注册  ' + fmtTime(user.created_at) + '\n最近活跃  ' + (user.last_active_at ? fmtTime(user.last_active_at) : '—') + (activeVersion ? '\n活跃时版本  ' + activeVersion : '') + '\n近7天活跃  ' + activeDaysText(user.active_days_7d) + '\n资料更新  ' + fmtTime(user.updated_at);
      if (user.avatar_url) document.getElementById('userDetailAvatar').src = user.avatar_url;
      backgroundURL = user.background_url || ''; loadBackground(); renderBirthday(user.zodiac_sign);
      profileReady = true; profileFields.disabled = false; detailSave.disabled = false; renderRestriction();
    } catch (error) {
      if (!currentDetail(userID, request) || controller.signal.aborted) return;
      detailModal.hide(); AdminUI.alert('读取用户资料失败: ' + error.message, 'danger');
    }
  }
  detailForm.addEventListener('submit', async function (event) {
    event.preventDefault();
    if (!profileReady || profileSaving || restrictionBusy || !AdminAccess.can('users', true) || !detailForm.reportValidity()) return;
    var userID = activeDetailUserID, request = userDetailRequest;
    profileSaving = true; profileFields.disabled = true; detailSave.disabled = true; detailSave.textContent = '保存中…'; renderRestriction();
    var payload = { nickname: detailForm.elements.nickname.value.trim(), gender: detailForm.elements.gender.value,
      birthday: detailForm.elements.birthday.value, signature: detailForm.elements.signature.value, is_internal: detailForm.elements.is_internal.checked };
    try {
      await userJSON(encodeURIComponent(userID), { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
      if (currentDetail(userID, request)) detailModal.hide();
      AdminUI.alert('用户资料已保存', 'success');
      loadPage().catch(function () { AdminUI.alert('资料已保存，列表刷新失败，请手动刷新', 'warning'); });
    } catch (error) { AdminUI.alert('保存失败: ' + error.message, 'danger'); }
    finally { if (currentDetail(userID, request)) { profileSaving = false; profileFields.disabled = !profileReady; detailSave.disabled = !profileReady; detailSave.textContent = '保存资料'; renderRestriction(); } }
  });
  tbody.addEventListener('click', function (event) {
    var button = event.target.closest('[data-action="detail"]');
    if (button) openUserDetail(button.dataset.userId);
  });

  refreshBtn.addEventListener('click', function () {
    cursorStack = [null];
    pageIndex = 0;
    loadPage();
  });
  function resetSearch() { cursorStack = [null]; pageIndex = 0; loadPage(); }
  searchBtn.addEventListener('click', resetSearch);
  titleFilter.addEventListener('keydown', function (event) { if (event.key === 'Enter') resetSearch(); });
  provinceFilter.addEventListener('change', function () { populateFilterCities(); resetSearch(); });
  [internalFilter, cityFilter, genderFilter, levelFilter, restrictedFilter].forEach(function (filter) {
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
    var request = ++listRequest;
    var params = new URLSearchParams();
    params.set('limit', String(pageSize));
    if (titleFilter.value.trim()) params.set('title', titleFilter.value.trim());
    if (internalFilter.value) params.set('is_internal', internalFilter.value);
    if (provinceFilter.value) params.set('province_code', provinceFilter.value);
    if (cityFilter.value) params.set('city_code', cityFilter.value);
    if (genderFilter.value) params.set('gender', genderFilter.value);
    if (levelFilter.value) params.set('level', levelFilter.value);
    if (restrictedFilter.value) params.set('restricted', restrictedFilter.value);
    var cur = cursorStack[pageIndex];
    if (cur) {
      params.set('cursor_created_at', cur.created_at);
      params.set('cursor_id', String(cur.id));
    }
    prevBtn.disabled = nextBtn.disabled = true;
    lastNextCursor = null;
    pageHint.textContent = '正在加载…';
    tbody.setAttribute('aria-busy', 'true');
    if (!tbody.querySelector('tr[data-user-id]')) tbody.innerHTML = '<tr><td colspan="8">正在加载用户…</td></tr>';
    try {
      var resp = await fetch('/admin/api/users?' + params.toString());
      var body = await resp.json();
      if (request !== listRequest) return;
      if (!resp.ok || body.code !== 0) throw new Error(body.error || String(resp.status));
      var data = body.data || {};
      var items = data.items || [];
      totalCount = data.total || 0;
      lastNextCursor = data.next_cursor || null;
      if (Array.isArray(data.level_options)) {
        var selectedLevel = levelFilter.value;
        levelFilter.replaceChildren(new Option('全部等级', ''));
        data.level_options.forEach(function (item) { levelFilter.add(new Option('Lv.' + item.level, String(item.level))); });
        levelFilter.value = selectedLevel;
      }

      var existing = new Map(Array.from(tbody.querySelectorAll('tr[data-user-id]')).map(function (row) { return [row.dataset.userId, row]; }));
      var rows = [];
      items.forEach(function (it) {
        var avatar = it.avatar_url
          ? '<img class="admin-user-avatar" src="' + esc(it.avatar_url) + '" alt="" />'
          : '<span class="admin-user-avatar admin-user-avatar-empty" aria-hidden="true">👤</span>';
        avatar = '<span class="admin-user-avatar-wrap">' + avatar + (it.is_internal ? '<span class="admin-user-inner-badge">inner</span>' : '') + '</span>';
        var id = String(it.id), tr = existing.get(id) || document.createElement('tr');
        var activeVersion = activeAppVersionText(it.last_active_app_version);
        var markup = '<td><div class="admin-user-identity">' + avatar + '<span class="admin-user-name"><span>' + esc(it.nickname) + '</span>' + (Number.isInteger(it.level) && it.level > 0 ? '<span class="admin-user-level">Lv.' + it.level + '</span>' : '') + '</span></div></td>' +
          '<td>' + esc(it.phone || '') + '</td>' +
          '<td>' + esc(genderText(it.gender)) + '</td>' +
          '<td><span class="admin-user-city-cell">' + esc(locationText(it)) + '</span></td>' +
          '<td>' + (it.follower_count == null ? '—' : esc(number(it.follower_count).toLocaleString())) + '</td>' +
          '<td><span class="admin-user-active-time">' + (it.last_active_at ? fmtTime(it.last_active_at) : '—') + '</span>' + (activeVersion ? '<small class="admin-user-active-version" title="最近活跃时版本：' + esc(activeVersion) + '">' + esc(activeVersion) + '</small>' : '') + '</td>' +
          '<td>' + activeDaysText(it.active_days_7d) + '</td>' +
          '<td><div class="admin-user-actions"><button data-action="detail" data-user-id="' + esc(it.id) + '">查看详情</button>' + '</div></td>';
        tr.dataset.userId = id;
        if (tr.dataset.markup !== markup) { tr.innerHTML = markup; tr.dataset.markup = markup; }
        rows.push(tr);
      });
      if (!rows.length) {
        var empty = document.createElement('tr'); empty.innerHTML = '<td colspan="8">暂无符合条件的用户</td>'; rows.push(empty);
      }
      var wanted = new Set(rows);
      Array.from(tbody.children).forEach(function (row) { if (!wanted.has(row)) row.remove(); });
      rows.forEach(function (row, index) { if (tbody.children[index] !== row) tbody.insertBefore(row, tbody.children[index] || null); });
      nextBtn.disabled = !lastNextCursor;
      pageHint.textContent = '第 ' + (pageIndex + 1) + ' 页 · 本页 ' + items.length + ' 个用户';
      totalHint.textContent = '共 ' + totalCount.toLocaleString() + ' 个用户';
    } catch (error) {
      if (request !== listRequest) return;
      if (!tbody.querySelector('tr[data-user-id]')) tbody.innerHTML = '<tr><td colspan="8">加载失败：' + esc(error.message) + '。请点击“刷新”重试。</td></tr>';
      pageHint.textContent = '加载失败：' + error.message + '。请点击“刷新”重试。';
    } finally {
      if (request === listRequest) {
        tbody.removeAttribute('aria-busy');
        prevBtn.disabled = pageIndex <= 0;
      }
    }
  }

  loadUserStatistics();
  loadCities();
  loadPage();
  window.addEventListener('resize', function () {
    Object.keys(userCharts).forEach(function (id) { userCharts[id].resize(); });
    if (userMap) userMap.resize();
  });
})();
