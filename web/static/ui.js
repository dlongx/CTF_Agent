export function escapeHTML(value) {
  return String(value).replace(/[&<>"']/g, (char) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  }[char]));
}

export function taskStatusText(status) {
  if (status === 'queued') return '排队中';
  if (status === 'running') return '运行中';
  if (status === 'completed') return '本轮结束';
  if (status === 'solved') return '已解出';
  if (status === 'failed') return '运行失败';
  return status || '-';
}

export function containerStateText(state) {
  if (state === 'running') return '正在解题';
  if (state === 'retained' || state === 'exited') return '已停止未销毁';
  if (state === 'missing') return 'Docker未找到';
  return state || '-';
}

export function statusBadge(status, id = '') {
  const idAttr = id ? ` id="${escapeHTML(id)}"` : '';
  return `<span${idAttr} class="badge ${escapeHTML(status || '')}">${escapeHTML(taskStatusText(status))}</span>`;
}

export function fmtTime(value) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}

export function fmtDuration(startValue, endValue) {
  if (!startValue) return '-';
  const start = new Date(startValue).getTime();
  const end = endValue ? new Date(endValue).getTime() : Date.now();
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return '-';
  const totalSeconds = Math.floor((end - start) / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  if (hours > 0) return `${hours}小时${minutes}分钟`;
  if (minutes > 0) return `${minutes}分钟`;
  return `${Math.max(totalSeconds, 0)}秒`;
}

export function text(value, fallback = '-') {
  return value === undefined || value === null || value === '' ? fallback : String(value);
}
