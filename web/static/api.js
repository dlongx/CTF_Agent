const defaultTimeoutMs = 20_000;

export async function fetchJSON(url, options = {}) {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), defaultTimeoutMs);
  try {
    const response = await fetch(url, { ...options, signal: options.signal || controller.signal });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(data.detail || data.error_code || `HTTP ${response.status}`);
    }
    return data;
  } catch (error) {
    if (error?.name === 'AbortError') {
      throw new Error('请求超时，请检查服务或Provider状态');
    }
    throw error;
  } finally {
    window.clearTimeout(timer);
  }
}
