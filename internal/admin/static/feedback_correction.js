(function () {
  var reasons = {
    '景点有误': '景点有误', 'Wrong scenic spot': '景点有误',
    '地区有误': '地区有误', 'Wrong area': '地区有误',
    '线路异常': '线路异常', 'Route issue': '线路异常',
    '数据异常': '数据异常', 'Data issue': '数据异常',
    '路况变化': '路况变化', 'Trail conditions': '路况变化',
    '其他问题': '其他问题', 'Other': '其他问题'
  };

  function parse(feedback) {
    var snapshot = feedback && feedback.correction;
    if (snapshot && /^NO\.[0-9A-Z]{8}$/.test(snapshot.track_id) && snapshot.option_name) {
      return { reason: snapshot.option_name, title: snapshot.track_title, trackID: snapshot.track_id, notes: snapshot.notes || '' };
    }
    // Only legacy submissions require matching the complete Android text header.
    var text = String(feedback && feedback.content || '').replace(/\r\n/g, '\n');
    var match = /^轨迹纠错：([^\n]+)\n轨迹：([^\n]+)\nID：(NO\.[0-9A-Z]{8})(?:\n\n([\s\S]*))?$/.exec(text) ||
      /^Route correction: ([^\n]+)\nRoute: ([^\n]+)\nID: (NO\.[0-9A-Z]{8})(?:\n\n([\s\S]*))?$/.exec(text);
    if (!match || !Object.prototype.hasOwnProperty.call(reasons, match[1])) return null;
    return { reason: reasons[match[1]], title: match[2], trackID: match[3], notes: match[4] || '' };
  }

  function trackURL(trackID) {
    return '/admin/tracks.html?track_id=' + encodeURIComponent(trackID);
  }

  window.AdminFeedbackCorrection = { parse: parse, trackURL: trackURL };
})();
