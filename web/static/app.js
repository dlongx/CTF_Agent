const $ = (selector) => document.querySelector(selector);
let allTasks = [];
let allContainers = [];
let cardRefreshTimeout;
let detailRefreshTimeout;
let containerRefreshTimeout;
let taskEventSource;
let dockerCardsPointerInside = false;
let pendingContainerRender = false;
const fastRefreshDelayMs = 50;
const activeRefreshIntervalMs = 1500;

function statusBadge(status, id = '') {
  const idAttr = id ? ` id="${escapeHTML(id)}"` : '';
  return `<span${idAttr} class="badge ${escapeHTML(status || '')}">${escapeHTML(taskStatusText(status))}</span>`;
}

function fmtTime(value) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}

function fmtDuration(startValue, endValue) {
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
  const activeCount = (counts.running || 0) + (counts.queued || 0);
  summary.textContent = `总计${allTasks.length}个任务 · 正在解题${activeCount} · 已解出${counts.solved || 0} · 未解出${counts.failed || 0}`;
  renderCards();
}

async function loadProviderSettings() {
  const select = $('#provider-format');
  const status = $('#provider-status');
  if (!select || !status) return;
  try {
    const settings = await fetchJSON('/api/settings/provider');
    select.innerHTML = (settings.options || []).map((option) => `
      <option value="${escapeHTML(option.format)}" ${option.active ? 'selected' : ''}>
        ${escapeHTML(option.label)}${option.configured ? '' : '（未配置）'}
      </option>
    `).join('');
    renderProviderStatus(settings);
  } catch (error) {
    status.textContent = error.message;
  }
}

function renderProviderStatus(settings) {
  const status = $('#provider-status');
  if (!status) return;
  const active = (settings.options || []).find((option) => option.active);
  if (!active) {
    status.textContent = '当前没有可用的模型接口配置。';
    return;
  }
  const configuredText = active.configured ? '已配置' : '缺少配置';
  const keyText = active.api_key_configured ? 'Key已配置' : 'Key未配置';
  const baseURLText = active.base_url_configured ? 'BaseURL已配置' : 'BaseURL未配置';
  status.textContent = `${active.label} · ${active.provider_npm} · ${active.model || '未设置模型'} · ${configuredText} · ${baseURLText} · ${keyText}`;
}

async function updateProviderSettings(event) {
  const select = event.currentTarget;
  const status = $('#provider-status');
  if (!status) return;
  status.textContent = '正在切换模型接口格式';
  select.disabled = true;
  try {
    const settings = await fetchJSON('/api/settings/provider', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ format: select.value }),
    });
    renderProviderStatus(settings);
    await loadProviderSettings();
  } catch (error) {
    status.textContent = error.message;
    await loadProviderSettings();
  } finally {
    select.disabled = false;
  }
}

function renderCards() {
  const cards = $('#task-cards');
  if (!cards) return;
  const tasks = filteredTasks();
  if (tasks.length === 0) {
    cards.innerHTML = '<p class="muted">没有符合筛选条件的任务。</p>';
    return;
  }
  cards.innerHTML = tasks.map((task) => {
    const writeupAction = task.status === 'solved' && task.has_writeup
      ? `<a class="button-link secondary" href="/api/tasks/${encodeURIComponent(task.id)}/writeup">下载WP</a>`
      : '';
    return `
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
          ${writeupAction}
          <button class="secondary copy-flag" data-flag="${escapeHTML(task.flag || '')}" ${task.flag ? '' : 'disabled'}>${task.flag ? '复制Flag' : '暂无Flag'}</button>
        </div>
      </article>
    `;
  }).join('');
  document.querySelectorAll('.copy-flag').forEach((button) => {
    button.addEventListener('click', copyFlag);
  });
}

async function loadContainers() {
  const cards = $('#docker-cards');
  if (!cards) return;
  const summary = $('#docker-summary');
  const payload = await fetchJSON('/api/containers');
  allContainers = payload.containers || [];
  const counts = allContainers.reduce((acc, item) => {
    acc[item.container_state] = (acc[item.container_state] || 0) + 1;
    return acc;
  }, {});
  const dockerStatus = payload.docker_available
    ? `未销毁容器${payload.live_count || 0}个`
    : `Docker不可用：${payload.docker_error || 'unknown'}`;
  summary.textContent = `${dockerStatus} · 正在解题${counts.running || 0} · 已停止${(counts.retained || 0) + (counts.exited || 0)}项`;
  if (dockerCardsPointerInside) {
    pendingContainerRender = true;
    return;
  }
  renderContainers();
}

function renderContainers() {
  const cards = $('#docker-cards');
  if (!cards) return;
  if (allContainers.length === 0) {
    cards.innerHTML = '<p class="muted">当前没有未销毁的ctf-agent任务容器。</p>';
    return;
  }
  cards.innerHTML = allContainers.map((item) => {
    const stateText = containerStateText(item.container_state);
    const canClose = item.docker_found;
    return `
      <article class="card docker-card">
        <a class="card-main" href="/tasks/${encodeURIComponent(item.task_id)}">
          <div class="card-head">
            <div>
              <h3>${escapeHTML(item.task_name)}</h3>
              <div class="card-subtitle">${escapeHTML(item.category || '-')} · ${escapeHTML(item.task_id)}</div>
            </div>
            <span class="badge ${escapeHTML(item.container_state)}">${stateText}</span>
          </div>
          <div class="docker-status-row">
            <span>任务:${escapeHTML(taskStatusText(item.task_status))}</span>
            <span>容器:${stateText}</span>
            <span>时长:${escapeHTML(fmtDuration(item.started_at, item.finished_at))}</span>
          </div>
        </a>
        <div class="card-actions">
          <a class="button-link secondary" href="/tasks/${encodeURIComponent(item.task_id)}">查看任务</a>
          <button class="secondary danger close-container-card" data-task-id="${escapeHTML(item.task_id)}" data-task-name="${escapeHTML(item.task_name)}" ${canClose ? '' : 'disabled'}>销毁容器</button>
        </div>
      </article>
    `;
  }).join('');
  document.querySelectorAll('.close-container-card').forEach((button) => {
    button.addEventListener('click', closeContainerFromCard);
  });
}

function containerStateText(state) {
  if (state === 'running') return '正在解题';
  if (state === 'retained') return '已停止未销毁';
  if (state === 'exited') return '已停止未销毁';
  if (state === 'missing') return 'Docker未找到';
  return state || '-';
}

function taskStatusText(status) {
  if (status === 'queued') return '正在解题';
  if (status === 'running') return '正在解题';
  if (status === 'solved') return '已解出';
  if (status === 'failed') return '未解出';
  return status || '-';
}

function filteredTasks() {
  const type = $('#filter-type')?.value || '';
  const status = $('#filter-status')?.value || '';
  const date = $('#filter-date')?.value || '';
  const now = new Date();
  return allTasks.filter((task) => {
    if (type && task.category !== type) return false;
    if (status === 'solved' && task.status !== 'solved') return false;
    if (status === 'active' && !['running', 'queued'].includes(task.status)) return false;
    if (status === 'unsolved' && task.status !== 'failed') return false;
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
  $('#task-writeup').innerHTML = task.status === 'solved' && task.has_writeup
    ? `<a href="/api/tasks/${encodeURIComponent(task.id)}/writeup">下载${escapeHTML(task.writeup_file_name || 'wp.md')}</a>`
    : '-';
  $('#task-container').textContent = task.container_kept
    ? `${task.container_name} 已保留`
    : (task.status === 'running' && task.container_name ? `${task.container_name} 运行中` : '未保留');
  updateOpenCodePanel(task);
  updateTerminalMessagePanel(task);
  $('#task-description').textContent = text(task.description, '');
}

function updateOpenCodePanel(task) {
  const field = $('#task-opencode');
  const status = task.opencode_status || 'unavailable';
  const message = task.opencode_message || '';
  if (field) {
    if (status === 'error') {
      field.innerHTML = `<span class="inline-error">OpenCode服务错误：${escapeHTML(message || '启动失败')}</span>`;
      return;
    }
    field.textContent = message || 'OpenCode终端不可用';
  }
}

function updateTerminalMessagePanel(task) {
  const panel = $('#message-panel');
  const input = $('#message-input');
  const button = $('#send-message');
  const state = $('#message-state');
  if (!panel || !input || !button || !state) return;
  const canSend = Boolean(task.can_send_message);
  panel.hidden = false;
  input.disabled = !canSend;
  button.disabled = !canSend;
  state.textContent = task.terminal_message_status || (canSend ? '可以继续发送' : '当前不可发送');
  input.placeholder = canSend
    ? '输入新的思路、约束或提示，发送后会继续同一个OpenCode终端会话'
    : (task.terminal_message_status || '当前不可发送消息');
  const messageStatus = $('#message-status');
  if (messageStatus && canSend) messageStatus.textContent = '';
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
  if (!confirmCloseContainer(button.dataset.taskName || taskID)) return;
  button.disabled = true;
  button.textContent = '销毁中';
  try {
    await fetchJSON(`/api/tasks/${encodeURIComponent(taskID)}/container/close`, { method: 'POST' });
    await loadCards();
    await loadContainers();
  } catch (error) {
    button.disabled = false;
    button.textContent = error.message;
  }
}

function confirmCloseContainer(name) {
  return window.confirm(`确认销毁${name}的容器吗？销毁后只能保留日志、已解出的WP和附件，不能继续这个运行现场。`);
}

async function sendMessage() {
  const shell = $('.detail-shell');
  const input = $('#message-input');
  const status = $('#message-status');
  if (!shell || !input || !status) return;
  const message = input.value.trim();
  if (!message) {
    status.textContent = '请输入消息内容';
    return;
  }
  const button = $('#send-message');
  if (button) button.disabled = true;
  status.textContent = '已提交，正在继续OpenCode终端会话';
  try {
    await fetchJSON(`/api/tasks/${encodeURIComponent(shell.dataset.taskId)}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message }),
    });
    input.value = '';
    await loadTaskDetail();
    await followLogs();
  } catch (error) {
    status.textContent = error.message;
  } finally {
    if (button) button.disabled = false;
    await loadTaskDetail().catch(() => {});
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
  }, fastRefreshDelayMs);
}

function scheduleDetailRefresh(taskID) {
  const shell = $('.detail-shell');
  if (!shell || shell.dataset.notFound === 'true') return;
  if (taskID && shell.dataset.taskId !== taskID) return;
  clearTimeout(detailRefreshTimeout);
  detailRefreshTimeout = setTimeout(() => {
    loadTaskDetail().catch(() => {});
  }, fastRefreshDelayMs);
}

function scheduleContainerRefresh() {
  if (!$('#docker-cards')) return;
  clearTimeout(containerRefreshTimeout);
  containerRefreshTimeout = setTimeout(() => {
    loadContainers().catch(() => {});
  }, fastRefreshDelayMs);
}

function setupDockerCardStability() {
  const cards = $('#docker-cards');
  if (!cards) return;
  cards.addEventListener('mouseenter', () => {
    dockerCardsPointerInside = true;
  });
  cards.addEventListener('mouseleave', () => {
    dockerCardsPointerInside = false;
    if (pendingContainerRender) {
      pendingContainerRender = false;
      renderContainers();
    }
  });
}

function setupActivePolling() {
  window.setInterval(() => {
    if ($('#task-cards') && allTasks.some((task) => task.status === 'running' || task.status === 'queued')) {
      loadCards().catch(() => {});
    }
    if ($('#docker-cards')) {
      loadContainers().catch(() => {});
    }
    const shell = $('.detail-shell');
    if (shell && shell.dataset.notFound !== 'true') {
      loadTaskDetail().catch(() => {});
    }
  }, activeRefreshIntervalMs);
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
    scheduleContainerRefresh();
  });
}

document.addEventListener('DOMContentLoaded', () => {
  $('#task-form')?.addEventListener('submit', submitTask);
  $('#provider-format')?.addEventListener('change', updateProviderSettings);
  $('#refresh')?.addEventListener('click', loadCards);
  $('#docker-refresh')?.addEventListener('click', loadContainers);
  $('#filter-type')?.addEventListener('change', renderCards);
  $('#filter-status')?.addEventListener('change', renderCards);
  $('#filter-date')?.addEventListener('change', renderCards);
  $('#load-logs')?.addEventListener('click', loadLogs);
  $('#follow-logs')?.addEventListener('click', followLogs);
  $('#clear-logs')?.addEventListener('click', () => {
    const logs = $('#logs');
    if (logs) logs.textContent = '';
  });
  $('#send-message')?.addEventListener('click', sendMessage);
  loadCards().catch((error) => {
    const cards = $('#task-cards');
    if (cards) cards.innerHTML = `<p class="muted">${escapeHTML(error.message)}</p>`;
  });
  loadProviderSettings().catch((error) => {
    const status = $('#provider-status');
    if (status) status.textContent = error.message;
  });
  loadContainers().catch((error) => {
    const cards = $('#docker-cards');
    if (cards) cards.innerHTML = `<p class="muted">${escapeHTML(error.message)}</p>`;
  });
  loadTaskDetail().catch((error) => {
    const logs = $('#logs');
    if (logs) logs.textContent = error.message;
  });
  setupTaskEvents();
  setupDropzone();
  setupDockerCardStability();
  setupActivePolling();
});
