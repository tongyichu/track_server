(function () {
  'use strict';
  const version = document.querySelector('meta[name="admin-assets-version"]');
  let chartsPromise;
  function url(path) {
    const result = new URL(path, location.origin);
    if (version && result.origin === location.origin && result.pathname.startsWith('/admin/static/')) {
      result.searchParams.set('v', version.content);
    }
    return result.href;
  }
  function loadCharts() {
    if (window.echarts) return Promise.resolve();
    if (!chartsPromise) {
      chartsPromise = new Promise(function (resolve, reject) {
        const script = document.createElement('script');
        script.src = url('/admin/static/vendor/echarts.min.js');
        script.async = true;
        script.onload = function () {
          if (window.echarts) resolve();
          else { script.remove(); chartsPromise = null; reject(new Error('图表组件暂不可用')); }
        };
        script.onerror = function () {
          script.remove(); chartsPromise = null;
          reject(new Error('图表库加载失败，请重试'));
        };
        document.head.appendChild(script);
      });
    }
    return chartsPromise;
  }
  window.AdminAssets = { url: url, loadCharts: loadCharts };
})();
