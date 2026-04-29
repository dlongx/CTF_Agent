const $ = (selector) => document.querySelector(selector);
let allTasks = [];
let cardRefreshTimeout;
let detailRefreshTimeout;
let taskEventSource;

function statusBadge(status, id = '') {
  const idAttr = id ? ` id="${escapeHTML(id)}"` : '';
  return `<span${idAttr} class="badge ${status || ''}">${status || '-'}</span>`;
}

function fmtTime(value) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}

function text(value, fallback = '-') {
  return value === undefined || value === null || value === '' ? fallback : String(value);
}

async function fetchJSON(url, options) {
  const response = await fetch(url, options);
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.detail || `HTTP ${response.status}`);
  }
  return data;
}

async function loadCards() {
  const cards = $('#task-cards');
  if (!cards) return;
  const summary = $('#summary');
  const payload = await fetchJSON('/api/tasks');
  allTasks = payload.tasks || [];
  const counts = allTasks.reduce((acc, task) => {
    acc[task.status] = (acc[task.status] || 0) + 1;
    return acc;
  }, {});
  summary.textContent = `总计${allTasks.length}个任务 · running ${counts.running || 0} · queued ${counts.queued || 0} · solved ${counts.solved || 0} · failed ${counts.failed || 0}`;
  renderCards();
}

function renderCards() {
  const cards = $('#task-cards');
  if (!cards) return;
  const tasks = filteredTasks();
  if (tasks.length === 0) {
    cards.innerHTML = '<p class="muted">没有符合筛选条件的任务。</p>';
    return;
  }
  cards.innerHTML = tasks.map((task) => `
    <article class="card">
      <a class="card-main" href="/tasks/${encodeURIComponent(task.id)}">
        <div class="card-head">
          <div>
            <h3>${escapeHTML(task.name)}</h3>
            <div class="card-subtitle">${escapeHTML(task.category || '-')} · 附件${task.attachment_count || 0}</div>
          </div>
          ${statusBadge(task.status)}
        </div>
        <div class="card-meta">
          <span>${task.flag ? '已捕获Flag' : '未捕获Flag'}</span>
          <span>${fmtTime(task.created_at)}</span>
        </div>
      </a>
      <div class="card-actions">
        <a class="button-link secondary ${task.has_writeup ? '' : 'disabled'}" href="${task.has_writeup ? `/api/tasks/${encodeURIComponent(task.id)}/writeup` : '#'}">${task.has_writeup ? '下载WP' : '暂无WP'}</a>
        <button class="secondary copy-flag" data-flag="${escapeHTML(task.flag || '')}" ${task.flag ? '' : 'disabled'}>${task.flag ? '复制Flag' : '暂无Flag'}</button>
        <button class="secondary danger close-container-card" data-task-id="${escapeHTML(task.id)}" ${task.container_kept ? '' : 'hidden'}>关闭容器</button>
      </div>
    </article>
  `).join('');
  document.querySelectorAll('.copy-flag').forEach((button) => {
    button.addEventListener('click', copyFlag);
  });
  document.querySelectorAll('.close-container-card').forEach((button) => {
    button.addEventListener('click', closeContainerFromCard);
  });
}

function filteredTasks() {
  const type = $('#filter-type')?.value || '';
  const status = $('#filter-status')?.value || '';
  const date = $('#filter-date')?.value || '';
  const now = new Date();
  return allTasks.filter((task) => {
    if (type && task.category !== type) return false;
    if (status === 'solved' && task.status !== 'solved') return false;
    if (status === 'unsolved' && task.status === 'solved') return false;
    if (['running', 'queued', 'failed'].includes(status) && task.status !== status) return false;
    if (date) {
      const created = new Date(task.created_at);
      const ageMs = now.getTime() - created.getTime();
      if (date === 'today' && created.toDateString() !== now.toDateString()) return false;
      if (date === '7d' && ageMs > 7 * 24 * 60 * 60 * 1000) return false;
      if (date === '30d' && ageMs > 30 * 24 * 60 * 60 * 1000) return false;
    }
    return true;
  });
}

async function submitTask(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const status = $('#form-status');
  status.textContent = '提交中';
  try {
    const task = await fetchJSON('/api/tasks', {
      method: 'POST',
      body: new FormData(form),
    });
    status.textContent = `已提交：${task.name || task.id}`;
    form.reset();
    updateFileSummary();
    await loadCards();
  } catch (error) {
    status.textContent = error.message;
  }
}

async function loadTaskDetail() {
  const shell = $('.detail-shell');
  if (!shell) return;
  if (shell.dataset.notFound === 'true') {
    $('#task-title').textContent = '任务不存在';
    $('#logs').textContent = '没有找到这个任务。';
    return;
  }
  const id = shell.dataset.taskId;
  const task = await fetchJSON(`/api/tasks/${encodeURIComponent(id)}`);
  $('#task-title').textContent = task.name;
  $('#task-status').outerHTML = statusBadge(task.status, 'task-status');
  $('#task-category').textContent = text(task.category);
  $('#task-target').textContent = text(task.target_ip);
  $('#task-attachments').textContent = text(task.attachment_count, '0');
  $('#task-flag').textContent = text(task.flag);
  $('#task-error').textContent = text(task.error);
  $('#task-last-step').textContent = text(task.last_step);
  $('#task-writeup').innerHTML = task.has_writeup
    ? `<a href="/api/tasks/${encodeURIComponent(task.id)}/writeup">下载${escapeHTML(task.writeup_file_name || 'wp.md')}</a>`
    : '-';
  $('#task-container').textContent = task.container_kept
    ? `${task.container_name} 已保留`
    : (task.status === 'running' && task.container_name ? `${task.container_name} 运行中` : '未保留');
  updateOpenCodePanel(task);
  $('#task-description').textContent = text(task.description, '');
  const hintPanel = $('#hint-panel');
  if (hintPanel) {
    hintPanel.hidden = !task.container_kept;
  }
}

function updateOpenCodePanel(task) {
  const field = $('#task-opencode');
  const hasEndpoint = Boolean(task.opencode_available && task.opencode_web_url);
  const hasSession = Boolean(hasEndpoint && task.opencode_session);
  if (field) {
    field.innerHTML = hasSession
      ? `<a class="button-link compact" href="${escapeHTML(task.opencode_web_url)}" target="_blank" rel="noopener">打开当前会话</a>`
      : hasEndpoint
        ? '等待OpenCode会话初始化'
      : '不可用';
  }
}

async function loadLogs() {
  const shell = $('.detail-shell');
  const logBox = $('#logs');
  if (!shell || !logBox) return;
  const id = shell.dataset.taskId;
  const payload = await fetchJSON(`/api/tasks/${encodeURIComponent(id)}/logs`);
  logBox.textContent = payload.logs || '';
  logBox.scrollTop = logBox.scrollHeight;
}

let socket;
async function followLogs() {
  const shell = $('.detail-shell');
  const logBox = $('#logs');
  if (!shell || !logBox) return;
  if (socket) socket.close();
  const id = shell.dataset.taskId;
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  logBox.textContent = '连接实时日志中...\n';
  socket = new WebSocket(`${protocol}//${window.location.host}/ws/tasks/${encodeURIComponent(id)}/logs`);
  socket.onmessage = (event) => {
    if (logBox.textContent === '连接实时日志中...\n') {
      logBox.textContent = '';
    }
    logBox.textContent += event.data;
    logBox.scrollTop = logBox.scrollHeight;
  };
  socket.onclose = () => {
    socket = undefined;
  };
}

function escapeHTML(value) {
  return String(value).replace(/[&<>"']/g, (char) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  }[char]));
}

async function copyFlag(event) {
  const button = event.currentTarget;
  const flag = button.dataset.flag || '';
  if (!flag) return;
  try {
    await navigator.clipboard.writeText(flag);
    button.textContent = '已复制';
    setTimeout(() => {
      button.textContent = '复制Flag';
    }, 1200);
  } catch {
    button.textContent = '复制失败';
    setTimeout(() => {
      button.textContent = '复制Flag';
    }, 1200);
  }
}

async function closeContainerFromCard(event) {
  event.preventDefault();
  event.stopPropagation();
  const button = event.currentTarget;
  const taskID = button.dataset.taskId;
  if (!taskID) return;
  button.disabled = true;
  button.textContent = '关闭中';
  try {
    await fetchJSON(`/api/tasks/${encodeURIComponent(taskID)}/container/close`, { method: 'POST' });
    await loadCards();
  } catch (error) {
    button.disabled = false;
    button.textContent = error.message;
  }
}

async function sendHint() {
  const shell = $('.detail-shell');
  const input = $('#hint-input');
  const status = $('#hint-status');
  if (!shell || !input || !status) return;
  const hint = input.value.trim();
  if (!hint) {
    status.textContent = '请输入提示内容';
    return;
  }
  status.textContent = '已提交，正在继续解题';
  try {
    await fetchJSON(`/api/tasks/${encodeURIComponent(shell.dataset.taskId)}/hints`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ hint }),
    });
    input.value = '';
    await loadTaskDetail();
    await followLogs();
  } catch (error) {
    status.textContent = error.message;
  }
}

async function closeContainerFromDetail() {
  const shell = $('.detail-shell');
  const status = $('#hint-status');
  if (!shell || !status) return;
  status.textContent = '正在关闭容器';
  try {
    await fetchJSON(`/api/tasks/${encodeURIComponent(shell.dataset.taskId)}/container/close`, { method: 'POST' });
    status.textContent = '容器已关闭';
    await loadTaskDetail();
  } catch (error) {
    status.textContent = error.message;
  }
}

function setupDropzone() {
  const dropzone = $('#dropzone');
  const input = $('#attachments');
  if (!dropzone || !input) return;
  input.addEventListener('change', updateFileSummary);
  ['dragenter', 'dragover'].forEach((name) => {
    dropzone.addEventListener(name, (event) => {
      event.preventDefault();
      dropzone.classList.add('dragover');
    });
  });
  ['dragleave', 'drop'].forEach((name) => {
    dropzone.addEventListener(name, (event) => {
      event.preventDefault();
      dropzone.classList.remove('dragover');
    });
  });
  dropzone.addEventListener('drop', (event) => {
    const files = event.dataTransfer?.files;
    if (!files || files.length === 0) return;
    input.files = files;
    updateFileSummary();
  });
}

function updateFileSummary() {
  const input = $('#attachments');
  const summary = $('#file-summary');
  if (!input || !summary) return;
  const files = Array.from(input.files || []);
  if (files.length === 0) {
    summary.textContent = '支持一次提交多个文件';
    return;
  }
  const names = files.slice(0, 3).map((file) => file.name).join('、');
  summary.textContent = files.length > 3 ? `${names}等${files.length}个文件` : names;
}

function scheduleCardRefresh() {
  if (!$('#task-cards')) return;
  clearTimeout(cardRefreshTimeout);
  cardRefreshTimeout = setTimeout(() => {
    loadCards().catch(() => {});
  }, 180);
}

function scheduleDetailRefresh(taskID) {
  const shell = $('.detail-shell');
  if (!shell || shell.dataset.notFound === 'true') return;
  if (taskID && shell.dataset.taskId !== taskID) return;
  clearTimeout(detailRefreshTimeout);
  detailRefreshTimeout = setTimeout(() => {
    loadTaskDetail().catch(() => {});
  }, 180);
}

function setupTaskEvents() {
  if (!window.EventSource || taskEventSource) return;
  taskEventSource = new EventSource('/api/events');
  taskEventSource.addEventListener('task', (event) => {
    let payload = {};
    try {
      payload = JSON.parse(event.data || '{}');
    } catch {
      return;
    }
    if (payload.type !== 'task_changed') return;
    scheduleCardRefresh();
    scheduleDetailRefresh(payload.task_id);
  });
}

document.addEventListener('DOMContentLoaded', () => {
  $('#task-form')?.addEventListener('submit', submitTask);
  $('#refresh')?.addEventListener('click', loadCards);
  $('#filter-type')?.addEventListener('change', renderCards);
  $('#filter-status')?.addEventListener('change', renderCards);
  $('#filter-date')?.addEventListener('change', renderCards);
  $('#load-logs')?.addEventListener('click', loadLogs);
  $('#follow-logs')?.addEventListener('click', followLogs);
  $('#clear-logs')?.addEventListener('click', () => {
    const logs = $('#logs');
    if (logs) logs.textContent = '';
  });
  $('#send-hint')?.addEventListener('click', sendHint);
  $('#close-container')?.addEventListener('click', closeContainerFromDetail);
  loadCards().catch((error) => {
    const cards = $('#task-cards');
    if (cards) cards.innerHTML = `<p class="muted">${escapeHTML(error.message)}</p>`;
  });
  loadTaskDetail().catch((error) => {
    const logs = $('#logs');
    if (logs) logs.textContent = error.message;
  });
  setupTaskEvents();
  setupDropzone();
});
