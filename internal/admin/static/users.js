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
    if (statsPanel.hidden) return;
    if (!window.echarts) { root.textContent = '图表组件暂不可用'; return; }
    loadProvinceGeo().then(function (geo) {
      if (statsPanel.hidden) return;
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
      userRegionPicker.setRegions(cities);
    } catch (error) {
      AdminUI.alert('城市列表加载失败: ' + (error.message || error), 'danger');
    }
  }
  var statisticsLoading = false, statisticsLoaded = false, statisticsData;
  var statsPanel = document.getElementById('userStatistics');
  var statsToggle = document.getElementById('userStatsToggle');
  var statisticsRefreshBtn = document.getElementById('refreshUserStatistics');
  statisticsRefreshBtn.addEventListener('click', loadUserStatistics);
  statsToggle.addEventListener('click', function () {
    statsPanel.hidden = !statsPanel.hidden;
    statsToggle.setAttribute('aria-expanded', String(!statsPanel.hidden));
    if (statsPanel.hidden) return;
    requestAnimationFrame(function () {
      if (statsPanel.hidden) return;
      if (statisticsLoaded) drawStatisticsCharts(statisticsData);
      else loadUserStatistics();
    });
  });
  function drawStatisticsCharts(data) {
    if (statsPanel.hidden) return;
    renderPie('userGenderChart', 'userGenderStats', data.gender, data.total_users);
    renderPie('userZodiacChart', 'userZodiacStats', data.zodiac, data.total_users);
    renderProvinceMap();
    renderPie('userAgeChart', 'userAgeStats', data.age, data.total_users, ageLabels);
    renderUserBars('userTrackOutputChart', data.track_output, trackOutputLabels, data.total_users);
    resizeUserCharts();
  }
  function resizeUserCharts() {
    if (statsPanel.hidden) return;
    Object.keys(userCharts).forEach(function (id) { userCharts[id].resize(); });
    if (userMap) userMap.resize();
  }
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
      await AdminAssets.loadCharts();
      renderProvinceStats(data.user_cities || [], false);
      drawStatisticsCharts(data);
      statisticsData = data;
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

  // Only successful requests replace the displayed page and its committed filters.
  var pageSize = 20;
  var listView = { filters: new URLSearchParams(), cursors: [null], index: 0, next: null };
  var listController, pendingListView, failedListView;

  var tbody = document.getElementById('userList');
  var prevBtn = document.getElementById('prevBtn');
  var nextBtn = document.getElementById('nextBtn');
  var refreshBtn = document.getElementById('refreshBtn');
  var searchBtn = document.getElementById('searchBtn');
  var extraFilters = document.getElementById('userExtraFilters');
  var extraFilterToggle = document.getElementById('userExtraFilterToggle');
  var extraFilterCount = document.getElementById('userExtraFilterCount');
  var titleFilter = document.getElementById('titleFilter');
  var internalFilter = document.getElementById('internalFilter');
  var provinceFilter = document.getElementById('provinceFilter');
  var cityFilter = document.getElementById('cityFilter');
  var userRegionPicker = AdminRegionPicker.enhance(provinceFilter, cityFilter, [], { label: '所在地区' });
  var genderFilter = document.getElementById('genderFilter');
  var levelFilter = document.getElementById('levelFilter');
  var restrictedFilter = document.getElementById('restrictedFilter');
  var pageHint = document.getElementById('pageHint');
  var totalHint = document.getElementById('totalHint');

  var detailRoot = document.getElementById('userDetailModal');
  var detailModal = new AdminComponents.Dialog(detailRoot, { backdrop: 'static' });
  var detailForm = document.getElementById('userDetailForm'), detailSave = document.getElementById('userDetailSave');
  var profileFields = document.getElementById('userProfileFields');
  var activeDetailUserID = '', userDetailRequest = 0, userDetailController = null;
  var profileReady = false, profileSaving = false, overviewLoading = false, permissionLoadedFor = '';
  var profileBaseline = null, discardPrompt = false, allowDiscard = false;
  var restriction = null, restrictionReady = false, restrictionBusy = false, restrictionMode = '';
  var restrictionTargetID = '';
  var restrictionDuration = document.getElementById('userRestrictionDuration');
  var restrictionDays = document.getElementById('userRestrictionDays');
  var profileLoadError = document.getElementById('userProfileLoadError');
  var detailStatus = document.getElementById('userDetailStatus');
  var detailCover = document.getElementById('userDetailCover');
  var backgroundURL = '', backgroundTimer = null;
  var background = document.getElementById('userDetailBackground');
  var avatar = document.getElementById('userDetailAvatar');
  var backgroundHint = document.getElementById('userBackgroundPlaceholder');
  var backgroundRetry = document.getElementById('userBackgroundRetry');
  var restrictionAction = document.getElementById('userRestrictionAction');
  var restrictionConfirm = document.getElementById('userRestrictionConfirm');
  var restrictionEditor = document.getElementById('userRestrictionEditor');
  var restrictionError = document.getElementById('userRestrictionError');
  function currentDetail(userID, request) { return userID === activeDetailUserID && request === userDetailRequest; }
  function detailMessage(message, error) {
    detailStatus.textContent = message;
    detailStatus.classList.toggle('text-danger', !!error);
  }
  function profileValues() {
    var fields = detailForm.elements;
    return { nickname: fields.nickname.value.trim(), gender: fields.gender.value,
      birthday: fields.birthday.value, signature: fields.signature.value, is_internal: fields.is_internal.checked };
  }
  function profileChanges() {
    if (!profileBaseline) return {};
    return Object.fromEntries(Object.entries(profileValues()).filter(function (field) {
      return field[1] !== profileBaseline[field[0]];
    }));
  }
  function hasProfileChanges() { return Object.keys(profileChanges()).length > 0; }
  function updateDetailActions() {
    var busy = profileSaving || restrictionBusy;
    var writable = AdminAccess.can('users', true);
    detailSave.closest('.modal-footer').hidden = !writable && !discardPrompt;
    detailSave.disabled = busy || !profileReady || discardPrompt || !hasProfileChanges() || !writable;
    detailSave.textContent = profileSaving ? '保存中…' : '保存资料';
    document.getElementById('userDetailCloseIcon').disabled = busy;
    document.getElementById('userDetailKeepEditing').hidden = !discardPrompt;
    document.getElementById('userDetailDiscard').hidden = !discardPrompt;
  }
  detailRoot.addEventListener('admin:dialog-closing', function (event) {
    if (profileSaving || restrictionBusy) {
      event.preventDefault();
      return;
    }
    if (!allowDiscard && hasProfileChanges()) {
      event.preventDefault();
      discardPrompt = true;
      detailMessage('资料有未保存的修改。');
      updateDetailActions();
      document.getElementById('userDetailKeepEditing').focus();
      return;
    }
    userDetailRequest++;
    if (userDetailController) userDetailController.abort();
    AdminUserPermissions.cancel();
    cancelBackground(); backgroundURL = '';
    cancelAvatar();
  });
  detailRoot.addEventListener('admin:dialog-closed', function () {
    activeDetailUserID = ''; profileReady = false; allowDiscard = false; profileBaseline = null;
  });
  document.getElementById('userDetailKeepEditing').addEventListener('click', function () {
    discardPrompt = false; detailMessage(''); updateDetailActions(); detailForm.elements.nickname.focus();
  });
  document.getElementById('userDetailDiscard').addEventListener('click', function () {
    if (profileSaving || restrictionBusy) return;
    allowDiscard = true; detailModal.hide();
  });
  detailForm.addEventListener('input', function () {
    discardPrompt = false; detailMessage(''); updateDetailActions();
  });
  function locationText(user) {
    var province = String(user.province_name || '').trim(), city = String(user.city_name || '').trim();
    return province && city && province !== city ? province + ' · ' + city : city || province || '-';
  }
  async function userJSON(path, options) {
    var response = await fetch('/admin/api/users/' + path, options);
    var payload = await response.json().catch(function () { return {}; });
    if (!response.ok || payload.code !== 0) {
      var error = new Error(payload.error || '请求失败：' + response.status);
      error.status = response.status;
      throw error;
    }
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
    detailCover.hidden = !backgroundURL;
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
  function cancelAvatar() {
    avatar.onload = null; avatar.onerror = null;
    avatar.removeAttribute('src');
  }
  function loadAvatar(url) {
    cancelAvatar();
    var placeholder = document.getElementById('userDetailAvatarPlaceholder');
    avatar.hidden = true; placeholder.hidden = false;
    if (!url) return;
    var image = avatar.cloneNode();
    avatar.replaceWith(image); avatar = image;
    var userID = activeDetailUserID, request = userDetailRequest;
    image.onload = function () {
      if (avatar !== image || !currentDetail(userID, request) || !image.naturalWidth) return;
      image.hidden = false; placeholder.hidden = true;
    };
    image.onerror = function () {
      if (avatar === image && currentDetail(userID, request)) cancelAvatar();
    };
    image.src = url;
  }
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
    restrictionConfirm.disabled = restrictionBusy || profileSaving;
    document.getElementById('userRestrictionCancel').disabled = restrictionBusy;
    document.getElementById('userRestrictionReason').disabled = restrictionBusy || !AdminAccess.can('users', true);
    restrictionDuration.disabled = restrictionBusy || !AdminAccess.can('users', true);
    renderRestrictionDuration();
    updateDetailActions();
  }
  function renderRestrictionDuration() {
    var byDays = restrictionDuration.value === 'days';
    document.getElementById('userRestrictionDaysField').hidden = !byDays;
    restrictionDays.disabled = restrictionDuration.disabled || !byDays;
  }
  restrictionDuration.addEventListener('change', renderRestrictionDuration);
  async function loadRestriction(userID, request, signal) {
    document.getElementById('userRestrictionRetry').hidden = true;
    try {
      var response = await fetch('/admin/api/users/' + encodeURIComponent(userID) + '/restrictions/current', { signal: signal });
      var payload = await response.json().catch(function () { return {}; });
      if (response.status !== 404 && (!response.ok || payload.code !== 0 || typeof payload.data?.id !== 'string')) {
        throw new Error(payload.error || '读取失败');
      }
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
    restrictionTargetID = restriction?.id || '';
    document.getElementById('userRestrictionCreateFields').hidden = restrictionMode !== 'create';
    document.getElementById('userRestrictionConfirmText').hidden = restrictionMode !== 'revoke';
    document.getElementById('userRestrictionReason').value = '';
    restrictionDuration.value = 'days'; restrictionDays.value = '7'; renderRestrictionDuration();
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
      if (!reason || Array.from(reason).length > 255) { restrictionError.textContent = '请填写 1–255 字的限制原因'; return; }
      payload = { reason: reason };
      if (restrictionDuration.value === 'days') {
        var days = restrictionDays.valueAsNumber;
        var end = new Date(Date.now() + days * 86400000);
        if (!restrictionDays.validity.valid || !Number.isSafeInteger(days) || !Number.isFinite(end.getTime()) || end.getUTCFullYear() > 9999) {
          restrictionError.textContent = '限制天数须为大于 0 的有效整数'; restrictionDays.focus(); return;
        }
        payload.expires_at = end.toISOString();
      }
    }
    restrictionBusy = true; restrictionError.textContent = '正在提交…'; renderRestriction();
    try {
      var path = create ? '/restrictions' : '/restrictions/current?restriction_id=' + encodeURIComponent(restrictionTargetID);
      await userJSON(encodeURIComponent(userID) + path, {
        method: create ? 'POST' : 'DELETE', signal: signal,
        headers: create ? { 'Content-Type': 'application/json' } : {}, body: create ? JSON.stringify(payload) : undefined
      });
      if (!currentDetail(userID, request)) return;
      restrictionEditor.hidden = true; restrictionAction.hidden = false; restrictionMode = '';
      restrictionError.textContent = '';
      AdminUI.alert(create ? '账号限制已生效' : '账号限制已解除', 'success');
      if ((pendingListView || listView).filters.has('restricted')) refreshVisibleList();
      restrictionReady = false;
      loadRestriction(userID, request, signal);
    } catch (error) {
      if (!currentDetail(userID, request) || signal.aborted) return;
      restrictionError.textContent = error.message;
      if (error.status === 409) {
        restrictionEditor.hidden = true; restrictionAction.hidden = false; restrictionMode = '';
        restrictionReady = false;
        loadRestriction(userID, request, signal);
      }
    } finally {
      if (currentDetail(userID, request)) { restrictionBusy = false; renderRestriction(); }
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
  document.getElementById('userProfileRetry').addEventListener('click', function () {
    if (activeDetailUserID) loadUserDetail(activeDetailUserID, true);
  });
  function renderProfile(user) {
    detailForm.elements.nickname.value = user.nickname || '';
    detailForm.elements.gender.value = user.gender || '';
    detailForm.elements.birthday.value = user.birthday || '';
    detailForm.elements.birthday.max = AdminUserZodiac.today();
    detailForm.elements.signature.value = user.signature || '';
    detailForm.elements.is_internal.checked = !!user.is_internal;
    profileBaseline = profileValues();
    document.getElementById('userDetailName').textContent = user.nickname || '未设置昵称';
    var meta = [
      ['手机号', user.phone || '—'],
      ['所在城市', user.city_name || user.province_name ? locationText(user) : '—'],
      ['注册', AdminTime.dateTime(user.created_at)],
      ['最近活跃', AdminTime.dateTime(user.last_active_at)],
      ['活跃时版本', activeAppVersionText(user.last_active_app_version)],
      ['近7天活跃', activeDaysText(user.active_days_7d)],
      ['资料更新', AdminTime.dateTime(user.updated_at)]
    ];
    document.getElementById('userDetailMeta').replaceChildren(...meta.filter(function (pair) { return pair[1]; }).map(function (pair) {
      var item = document.createElement('div'), term = document.createElement('dt'), value = document.createElement('dd');
      term.textContent = pair[0]; value.textContent = pair[1];
      item.append(term, value);
      return item;
    }));
    renderBirthday(user.zodiac_sign);
  }
  async function loadUserDetail(userID, retry) {
    if (!retry && activeDetailUserID === userID && detailRoot.classList.contains('show')) return;
    var request = ++userDetailRequest;
    if (userDetailController) userDetailController.abort();
    var controller = userDetailController = new AbortController();
    activeDetailUserID = userID; profileReady = false; profileSaving = false; overviewLoading = false; permissionLoadedFor = '';
    profileBaseline = null; discardPrompt = allowDiscard = false;
    detailMessage(''); profileLoadError.hidden = true;
    restriction = null; restrictionReady = false; restrictionBusy = false; restrictionMode = '';
    detailForm.reset(); profileFields.disabled = true;
    document.getElementById('userDetailName').textContent = '加载中…';
    document.getElementById('userDetailMeta').textContent = ''; document.getElementById('userDetailID').textContent = 'ID ' + userID;
    loadAvatar(''); document.getElementById('userDetailLevel').hidden = true;
    ['userFollowingCount','userFollowerCount','userReceivedLikeCount'].forEach(function (id) { document.getElementById(id).textContent = '—'; });
    document.getElementById('userPermissionSection').open = false; document.getElementById('userPermissionSnapshots').replaceChildren();
    restrictionEditor.hidden = true; restrictionAction.hidden = false; restrictionError.textContent = '';
    renderRestriction(); document.getElementById('userRestrictionStatus').textContent = '读取中';
    backgroundURL = ''; loadBackground(); renderBirthday(); detailModal.show();
    loadOverview(userID, request, controller.signal); loadRestriction(userID, request, controller.signal);
    try {
      var user = await userJSON(encodeURIComponent(userID), { signal: controller.signal });
      if (!currentDetail(userID, request)) return;
      renderProfile(user);
      loadAvatar(user.avatar_url);
      backgroundURL = user.background_url || ''; loadBackground();
      profileReady = true; profileFields.disabled = false; renderRestriction();
    } catch (error) {
      if (!currentDetail(userID, request) || controller.signal.aborted) return;
      document.getElementById('userDetailName').textContent = '用户资料';
      profileLoadError.hidden = false;
    }
  }
  detailForm.addEventListener('submit', async function (event) {
    event.preventDefault();
    if (!profileReady || profileSaving || restrictionBusy || discardPrompt || !AdminAccess.can('users', true) || !detailForm.reportValidity()) return;
    var payload = profileChanges();
    if (!Object.keys(payload).length) return;
    if (payload.nickname === '') {
      detailMessage('请输入昵称', true); detailForm.elements.nickname.focus(); return;
    }
    var userID = activeDetailUserID, request = userDetailRequest;
    var saved = false;
    profileSaving = true; profileFields.disabled = true; detailMessage(''); renderRestriction();
    try {
      await userJSON(encodeURIComponent(userID), { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
      if (currentDetail(userID, request)) { profileBaseline = profileValues(); saved = true; }
    } catch (error) {
      if (currentDetail(userID, request)) detailMessage('保存失败：' + error.message, true);
    } finally {
      if (currentDetail(userID, request)) { profileSaving = false; profileFields.disabled = !profileReady; renderRestriction(); }
    }
    if (!saved) return;
    detailModal.hide();
    AdminUI.alert('资料已保存', 'success');
    refreshVisibleList().then(function (updated) {
      if (updated === false) AdminUI.alert('资料已保存，用户列表刷新失败。', 'warning');
    });
  });
  tbody.addEventListener('click', function (event) {
    var button = event.target.closest('[data-action="detail"]');
    if (button) openUserDetail(button.dataset.userId);
  });

  var liveSearch = AdminSearch.bind([titleFilter], resetSearch, function () {
    listController?.abort();
    prevBtn.disabled = nextBtn.disabled = true;
  });
  refreshBtn.addEventListener('click', function () {
    if (liveSearch.pending) resetSearch();
    else loadPage(failedListView || listView);
  });
  function resetSearch() {
    liveSearch.cancel();
    return loadPage({ filters: readFilters(), cursors: [null], index: 0, next: null });
  }
  extraFilterToggle.addEventListener('click', function () {
    extraFilters.hidden = !extraFilters.hidden;
    extraFilterToggle.setAttribute('aria-expanded', String(!extraFilters.hidden));
    if (extraFilters.hidden) userRegionPicker.close();
  });
  searchBtn.addEventListener('click', resetSearch);
  titleFilter.addEventListener('keydown', function (event) {
    if (event.key === 'Enter' && !event.isComposing) resetSearch();
  });
  provinceFilter.addEventListener('change', function () { populateFilterCities(); resetSearch(); });
  [internalFilter, cityFilter, genderFilter, levelFilter, restrictedFilter].forEach(function (filter) {
    filter.addEventListener('change', resetSearch);
  });
  prevBtn.addEventListener('click', function () {
    if (listView.index <= 0 || pendingListView) return;
    var index = listView.index - 1;
    loadPage({ filters: listView.filters, cursors: listView.cursors.slice(0, index + 1), index: index });
  });
  nextBtn.addEventListener('click', function () {
    if (!listView.next || pendingListView) return;
    loadPage({ filters: listView.filters, cursors: listView.cursors.concat(listView.next), index: listView.index + 1 });
  });

  function readFilters() {
    var params = new URLSearchParams();
    if (titleFilter.value.trim()) params.set('title', titleFilter.value.trim());
    if (internalFilter.value) params.set('is_internal', internalFilter.value);
    if (provinceFilter.value) params.set('province_code', provinceFilter.value);
    if (cityFilter.value) params.set('city_code', cityFilter.value);
    if (genderFilter.value) params.set('gender', genderFilter.value);
    if (levelFilter.value) params.set('level', levelFilter.value);
    if (restrictedFilter.value) params.set('restricted', restrictedFilter.value);
    var extraCount = [internalFilter.value, cityFilter.value || provinceFilter.value, genderFilter.value, levelFilter.value, restrictedFilter.value].filter(Boolean).length;
    extraFilterCount.textContent = extraCount;
    extraFilterCount.hidden = extraCount === 0;
    extraFilterToggle.setAttribute('aria-label', extraCount ? '筛选，已选 ' + extraCount + ' 项' : '筛选');
    return params;
  }

  function refreshVisibleList() {
    if (liveSearch.pending) return resetSearch();
    var view = pendingListView || listView;
    return loadPage(view);
  }

  async function loadPage(view) {
    listController?.abort();
    var controller = listController = new AbortController();
    pendingListView = view;
    failedListView = null;
    var params = new URLSearchParams(view.filters);
    params.set('limit', String(pageSize));
    var cur = view.cursors[view.index];
    if (cur) {
      params.set('cursor_created_at', cur.created_at);
      params.set('cursor_id', String(cur.id));
    }
    prevBtn.disabled = nextBtn.disabled = refreshBtn.disabled = true;
    pageHint.textContent = '正在加载…';
    tbody.setAttribute('aria-busy', 'true');
    if (!tbody.querySelector('tr[data-user-id]')) tbody.innerHTML = '<tr><td colspan="7">正在加载用户…</td></tr>';
    try {
      var resp = await fetch('/admin/api/users?' + params.toString(), { signal: controller.signal });
      var body = await resp.json();
      if (listController !== controller || controller.signal.aborted) return;
      if (!resp.ok || body.code !== 0) throw new Error(body.error || String(resp.status));
      var data = body.data || {};
      var items = data.items || [];
      var totalCount = data.total || 0;
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
        var markup = '<td><div class="admin-user-identity">' + avatar + '<div class="admin-user-name"><button class="btn btn-link admin-user-name-link" type="button" data-action="detail" data-user-id="' + esc(it.id) + '" title="查看用户详情">' + esc(it.nickname || '未设置昵称') + '</button>' + (Number.isInteger(it.level) && it.level > 0 ? '<span class="admin-user-level">Lv.' + it.level + '</span>' : '') + '</div></div></td>' +
          '<td>' + esc(it.phone || '') + '</td>' +
          '<td>' + esc(genderText(it.gender)) + '</td>' +
          '<td><span class="admin-user-city-cell">' + esc(locationText(it)) + '</span></td>' +
          '<td>' + (it.follower_count == null ? '—' : esc(number(it.follower_count).toLocaleString())) + '</td>' +
          '<td><span class="admin-user-active-time">' + (it.last_active_at ? fmtTime(it.last_active_at) : '—') + '</span>' + (activeVersion ? '<small class="admin-user-active-version" title="最近活跃时版本：' + esc(activeVersion) + '">' + esc(activeVersion) + '</small>' : '') + '</td>' +
          '<td>' + activeDaysText(it.active_days_7d) + '</td>';
        tr.dataset.userId = id;
        if (tr.dataset.markup !== markup) { tr.innerHTML = markup; tr.dataset.markup = markup; }
        rows.push(tr);
      });
      if (!rows.length) {
        var empty = document.createElement('tr'); empty.innerHTML = '<td colspan="7">暂无符合条件的用户</td>'; rows.push(empty);
      }
      var wanted = new Set(rows);
      Array.from(tbody.children).forEach(function (row) { if (!wanted.has(row)) row.remove(); });
      rows.forEach(function (row, index) { if (tbody.children[index] !== row) tbody.insertBefore(row, tbody.children[index] || null); });
      listView = { filters: view.filters, cursors: view.cursors, index: view.index, next: data.next_cursor || null };
      pageHint.textContent = '第 ' + (view.index + 1) + ' 页 · 本页 ' + items.length + ' 个用户';
      totalHint.textContent = '共 ' + totalCount.toLocaleString() + ' 个用户';
      return true;
    } catch (error) {
      if (listController !== controller || controller.signal.aborted) return;
      failedListView = view;
      var hasRows = !!tbody.querySelector('tr[data-user-id]');
      if (!hasRows) tbody.innerHTML = '<tr><td colspan="7">加载失败：' + esc(error.message) + '。请点击“刷新”重试。</td></tr>';
      pageHint.textContent = hasRows ? '当前显示上次加载的结果。请点击“刷新”重试。' : '加载失败，请点击“刷新”重试。';
      return false;
    } finally {
      if (listController === controller && !controller.signal.aborted) {
        pendingListView = null;
        tbody.removeAttribute('aria-busy');
        prevBtn.disabled = listView.index <= 0;
        nextBtn.disabled = !listView.next;
        refreshBtn.disabled = false;
      }
    }
  }
  loadCities();
  resetSearch();
  window.addEventListener('resize', resizeUserCharts);
})();
