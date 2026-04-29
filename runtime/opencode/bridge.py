"""Container-side bridge between the CTF platform and OpenCode Web.

This bridge is intentionally lightweight. It starts an OpenCode Web server
inside the task container, sends a challenge prompt, and prints server output to
stdout so the existing Go API -> WebSocket -> Xterm pipeline can stay intact.
"""

from __future__ import annotations

import json
import os
import subprocess
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any

MAX_SKILL_CHARS = 12000


@dataclass(frozen=True)
class BridgeConfig:
    """Runtime settings for the OpenCode bridge."""

    name: str
    category: str
    description: str
    target_ip: str
    attachment_dir: Path
    skills_dir: Path
    skill_ids: tuple[str, ...]
    server_url: str
    provider_id: str
    provider_name: str
    provider_npm: str
    base_url: str
    api_key: str
    model: str
    user_hint: str


def log(message: str) -> None:
    """Emit one terminal line immediately."""
    print(message, flush=True)


def read_config() -> BridgeConfig:
    """Read task metadata from environment variables."""
    return BridgeConfig(
        name=os.getenv("CHALLENGE_NAME", "unnamed"),
        category=os.getenv("CHALLENGE_TYPE", "misc"),
        description=os.getenv("CHALLENGE_DESC", ""),
        target_ip=os.getenv("TARGET_IP", ""),
        attachment_dir=Path(os.getenv("ATTACHMENT_DIR", "/attachments")),
        skills_dir=Path(os.getenv("CTF_AGENT_SKILLS_DIR", "/skills")),
        skill_ids=parse_skill_ids(os.getenv("CTF_AGENT_SKILL_IDS", "")),
        server_url=os.getenv("OPENCODE_SERVER_URL", "http://127.0.0.1:4096").rstrip("/"),
        provider_id=os.getenv("OPENCODE_PROVIDER_ID", "").strip(),
        provider_name=os.getenv("OPENCODE_PROVIDER_NAME", "CTF Model Gateway").strip(),
        provider_npm=os.getenv("OPENCODE_PROVIDER_NPM", "@ai-sdk/openai-compatible").strip(),
        base_url=os.getenv("OPENCODE_BASE_URL", "").strip().rstrip("/"),
        api_key=os.getenv("OPENCODE_API_KEY", "").strip(),
        model=os.getenv("OPENCODE_MODEL", "").strip(),
        user_hint=os.getenv("CTF_AGENT_USER_HINT", "").strip(),
    )


def parse_skill_ids(raw: str) -> tuple[str, ...]:
    """Parse comma-separated skill ids from the host dispatcher."""
    items = []
    for item in raw.split(","):
        normalized = normalize_category(item)
        if normalized and normalized not in items:
            items.append(normalized)
    return tuple(items)


def request_json(
    method: str,
    url: str,
    payload: dict[str, Any] | None = None,
    *,
    timeout_seconds: int | None = 30,
) -> dict[str, Any]:
    """Send an HTTP JSON request to OpenCode Server."""
    data = None if payload is None else json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=data,
        headers={"Content-Type": "application/json"},
        method=method,
    )
    try:
        if timeout_seconds is None:
            response_context = urllib.request.urlopen(request)
        else:
            response_context = urllib.request.urlopen(request, timeout=timeout_seconds)
        with response_context as response:
            body = response.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"HTTP {exc.code} from {url}: {body[:2000]}") from exc
    if not body:
        return {}
    try:
        return json.loads(body)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"non-JSON response from {url}: {body[:2000]}") from exc


def start_server() -> subprocess.Popen[str]:
    """Start OpenCode Web bound to the container network interface."""
    workspace = Path("/workspace")
    workspace.mkdir(parents=True, exist_ok=True)
    runtime_tmp = Path("/workspace/.tmp")
    runtime_tmp.mkdir(parents=True, exist_ok=True)
    os.environ["TMPDIR"] = str(runtime_tmp)
    log_file = Path("/workspace/opencode-web.log").open("a", encoding="utf-8")
    return subprocess.Popen(
        [
            "opencode",
            "web",
            "--hostname",
            "0.0.0.0",
            "--port",
            "4096",
            "--print-logs",
            "--log-level",
            os.getenv("OPENCODE_LOG_LEVEL", "WARN"),
        ],
        cwd=str(workspace),
        text=True,
        stdout=log_file,
        stderr=subprocess.STDOUT,
    )


def configure_opencode(config: BridgeConfig) -> None:
    """Write a transient OpenCode project config when model env vars are provided."""
    if not config.provider_id and not config.model:
        raise RuntimeError(
            "OpenCode model is not configured. Set OPENCODE_PROVIDER_ID, "
            "OPENCODE_BASE_URL, OPENCODE_API_KEY, and OPENCODE_MODEL before starting backend."
        )
    if not config.provider_id or not config.model:
        raise RuntimeError("OPENCODE_PROVIDER_ID and OPENCODE_MODEL must be set together")
    if not config.base_url:
        raise RuntimeError("OPENCODE_BASE_URL is required when OPENCODE_MODEL is set")
    if not config.api_key:
        raise RuntimeError("OPENCODE_API_KEY is required when OPENCODE_MODEL is set")

    provider_config = {
        "npm": config.provider_npm,
        "name": config.provider_name or config.provider_id,
        "options": {
            "baseURL": config.base_url,
            "apiKey": config.api_key,
        },
        "models": {
            config.model: {
                "name": config.model,
            },
        },
    }
    opencode_config = {
        "$schema": "https://opencode.ai/config.json",
        "model": f"{config.provider_id}/{config.model}",
        "small_model": f"{config.provider_id}/{config.model}",
        "permission": "allow",
        "provider": {
            config.provider_id: provider_config,
        },
    }
    Path("/workspace/opencode.json").write_text(
        json.dumps(opencode_config, ensure_ascii=False, indent=2),
        encoding="utf-8",
    )
    log(
        "Thought: wrote OpenCode provider config "
        f"provider={config.provider_id!r} model={config.model!r} base_url={config.base_url!r}."
    )


def wait_for_server(config: BridgeConfig, process: subprocess.Popen[str]) -> None:
    """Wait until OpenCode Server responds or the process exits."""
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if process.poll() is not None:
            output = process.stdout.read() if process.stdout else ""
            raise RuntimeError(f"opencode server exited early: {output}")
        try:
            request_json("GET", f"{config.server_url}/global/health", timeout_seconds=5)
            return
        except (OSError, urllib.error.URLError):
            time.sleep(0.5)
    raise RuntimeError("opencode server did not become ready")


def category_guidance(category: str) -> str:
    """Return focused solving guidance for the submitted CTF category."""
    normalized = normalize_category(category)
    guidance = {
        "web": (
            "Web challenge workflow: inspect source files first, identify routes, templates, "
            "auth/session logic, deserialization, SSRF, SQL injection, command injection, "
            "path traversal, upload handling, and hardcoded secrets. If a target IP is provided, "
            "probe it carefully with curl or similar tools from inside the container."
        ),
        "pwn": (
            "Pwn challenge workflow: identify architecture and protections, inspect strings and "
            "symbols, run the binary locally when possible, look for overflow, format string, "
            "heap, ROP, ret2libc, and seccomp constraints. Prefer reproducible exploit scripts."
        ),
        "crypto": (
            "Crypto challenge workflow: read all scripts and outputs, identify primitives and "
            "parameters, check weak randomness, small exponents, reused nonces, padding oracles, "
            "linear relations, bad key derivation, and encoding layers before brute force."
        ),
        "reverse": (
            "Reverse challenge workflow: inspect file type, strings, symbols, bytecode, packed or "
            "obfuscated sections, input validation paths, constants, and custom VMs. Recover the "
            "flag algorithm before guessing."
        ),
        "forensics": (
            "Forensics challenge workflow: identify file formats, metadata, embedded files, "
            "archives, steganography, PCAP streams, logs, disk images, and memory artifacts. "
            "Preserve evidence and extract flags from decoded artifacts."
        ),
        "misc": (
            "Misc challenge workflow: enumerate files, detect encodings and formats, inspect text "
            "and binary previews, try common CTF transformations, scripting, constraint solving, "
            "and archive extraction as needed."
        ),
    }
    return guidance.get(normalized, guidance["misc"])


def normalize_category(category: str) -> str:
    """Normalize category and skill ids."""
    return category.strip().lower().replace("-", "_")


def skill_ids_for_config(config: BridgeConfig) -> tuple[str, ...]:
    """Return skill ids in priority order."""
    if config.skill_ids:
        return config.skill_ids
    category = normalize_category(config.category)
    if category in {"crypto", "web", "pwn", "reverse", "forensics", "misc"}:
        return (category,)
    return ("misc",)


def read_skill_text(config: BridgeConfig) -> str:
    """Read active skill markdown files mounted from the host."""
    blocks = []
    loaded = []
    for skill_id in skill_ids_for_config(config):
        skill_path = config.skills_dir / f"{skill_id}.md"
        try:
            text = skill_path.read_text(encoding="utf-8")
        except OSError as exc:
            blocks.append(
                f"--- skill: {skill_id} ---\n"
                f"Skill file was not available at {skill_path}: {exc}\n"
            )
            continue
        loaded.append(skill_id)
        if len(text) > MAX_SKILL_CHARS:
            text = text[:MAX_SKILL_CHARS] + "\n\n[skill truncated]\n"
        blocks.append(f"--- skill: {skill_id} ---\n{text.strip()}\n")
    if loaded:
        log(
            "Observation: loaded CTF skill(s)="
            f"{','.join(loaded)} from {config.skills_dir}"
        )
    return "\n".join(blocks)


def build_prompt(config: BridgeConfig) -> str:
    """Build the initial CTF solving prompt for OpenCode."""
    files = []
    if config.attachment_dir.exists():
        files = [str(path) for path in sorted(config.attachment_dir.rglob("*")) if path.is_file()]
    skill_text = read_skill_text(config)

    return (
        "You are solving a CTF challenge inside an isolated Docker container.\n"
        "Use shell and file tools if available. Keep output concise and print the final flag.\n"
        "Only work on the challenge files mounted at /attachments and the provided target.\n"
        "Do not ask the user for interactive confirmation; "
        "solve autonomously within this container.\n"
        "When you summarize the solution, write all narrative explanations in Simplified Chinese. "
        "Keep commands, source code, file names, technical identifiers, formulas, and flags unchanged. "
        "Do not write English writeup prose unless it is part of source code or command output.\n\n"
        "Your final response must end with one standalone final line containing only the exact flag. "
        "Do not wrap that final line in Markdown, do not prefix it with 'Flag:', and do not add any text "
        "after it. The platform will treat this last non-empty line as the captured flag, regardless of "
        "flag wrapper format.\n\n"
        f"Name: {config.name}\n"
        f"Category: {config.category}\n"
        f"Target IP: {config.target_ip or 'not provided'}\n"
        f"Description: {config.description}\n"
        f"Category guidance: {category_guidance(config.category)}\n"
        + (f"User hint for this continuation: {config.user_hint}\n" if config.user_hint else "")
        + "Active CTF skills:\n"
        f"{skill_text or 'No skill files loaded.'}\n\n"
        "Attachments are mounted read-only at /attachments.\n"
        "Attachment files:\n"
        + ("\n".join(files) if files else "No files found.")
    )


def create_session(config: BridgeConfig) -> str:
    """Create an OpenCode session and return its id."""
    response = request_json("POST", f"{config.server_url}/session", {"title": config.name})
    session_id = (
        response.get("id")
        or response.get("sessionID")
        or response.get("session", {}).get("id")
    )
    if not session_id:
        raise RuntimeError(f"could not determine session id from response: {response}")
    return str(session_id)


def send_prompt(config: BridgeConfig, session_id: str, prompt: str) -> dict[str, Any]:
    """Send a user text part to OpenCode."""
    log(
        "Action: send prompt through OpenCode "
        f"model={config.provider_id}/{config.model} timeout=disabled"
    )
    return request_json(
        "POST",
        f"{config.server_url}/session/{session_id}/message",
        {"parts": [{"type": "text", "text": prompt}]},
        timeout_seconds=None,
    )


def export_session(session_id: str) -> dict[str, Any]:
    """Export OpenCode session JSON through CLI for robust response extraction."""
    result = subprocess.run(
        ["opencode", "export", session_id],
        cwd="/workspace",
        text=True,
        capture_output=True,
        timeout=20,
        check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip() or "opencode export failed")
    text = result.stdout.strip()
    if not text:
        return {}
    return json.loads(text)


def wait_for_session_text(config: BridgeConfig, session_id: str) -> str:
    """Poll exported session data until assistant text or final tool output appears."""
    last_text = ""
    stable_count = 0
    while True:
        time.sleep(2)
        try:
            exported = export_session(session_id)
        except Exception as exc:
            last_text = f"Unable to export session yet: {exc}"
            continue
        text = extract_text(exported)
        if text and text == last_text:
            stable_count += 1
        elif text:
            stable_count = 0
            last_text = text
            log_delta_preview(text)
        if last_text and stable_count >= 2:
            return last_text
    return last_text


def log_delta_preview(text: str) -> None:
    """Print only the newest readable chunk while polling."""
    clean = text.strip()
    if not clean:
        return
    tail = clean[-1200:]
    log("Observation: assistant output updated:")
    log(tail)


def extract_text(value: Any) -> str:
    """Extract readable assistant/tool text from exported OpenCode JSON."""
    assistant_blocks: list[str] = []
    fallback_blocks: list[str] = []

    assistant_roles = {"assistant", "model", "agent"}
    ignored_roles = {"user", "system"}

    def append_block(text: str, role: str | None) -> None:
        clean = text.strip()
        if not clean:
            return
        if role in assistant_roles:
            assistant_blocks.append(clean)
            return
        if role not in ignored_roles and not looks_like_prompt_echo(clean):
            fallback_blocks.append(clean)

    def walk(node: Any, inherited_role: str | None = None) -> None:
        if isinstance(node, dict):
            role_value = node.get("role") or node.get("speaker")
            role = str(role_value).strip().lower() if isinstance(role_value, str) else inherited_role
            text = node.get("text") or node.get("content") or node.get("message")
            if isinstance(text, str) and text.strip():
                append_block(text, role)
            for key in ("output", "result", "error"):
                item = node.get(key)
                if isinstance(item, str) and item.strip():
                    append_block(item, role)
            for item in node.values():
                walk(item, role)
        elif isinstance(node, list):
            for item in node:
                walk(item, inherited_role)

    walk(value)
    blocks = assistant_blocks or fallback_blocks
    deduped = []
    seen = set()
    for block in blocks:
        if looks_like_prompt_echo(block):
            continue
        if block in seen:
            continue
        seen.add(block)
        deduped.append(block)
    return "\n\n".join(deduped)


def looks_like_prompt_echo(text: str) -> bool:
    """Return true for exported user prompt or mounted skill text."""
    normalized = text.strip().lower()
    if not normalized:
        return False
    markers = (
        "active ctf skills:",
        "attachments are mounted read-only at /attachments",
        "# ctf miscellaneous",
        "quick reference for miscellaneous ctf challenges",
        "allowed-tools:",
        "compatibility:",
        "license: mit",
        "manual install:",
        "quick start commands",
        "[skill truncated]",
        "you are solving a ctf challenge",
    )
    return sum(1 for marker in markers if marker in normalized) >= 2


def run_bridge() -> int:
    """Run one OpenCode-backed challenge attempt."""
    config = read_config()
    log("\x1b[36m[agent] Phase 5 OpenCode bridge started\x1b[0m")
    log(f"Thought: challenge={config.name!r} category={config.category!r}")
    log("Action: start opencode web on 0.0.0.0:4096")
    process: subprocess.Popen[str] | None = None

    try:
        configure_opencode(config)
        process = start_server()
        wait_for_server(config, process)
        log("Observation: OpenCode Web is ready.")
        session_id = create_session(config)
        log(f"Observation: OpenCode session={session_id}")
        send_prompt(config, session_id, build_prompt(config))
        final_text = wait_for_session_text(config, session_id)
        if final_text:
            log("Observation: final readable OpenCode output:")
            log(final_text[-12000:])
        else:
            raise RuntimeError("OpenCode finished without readable assistant output")
    except Exception as exc:
        log(f"Final: opencode bridge failed: {exc}")
        return 1
    log("Final: opencode bridge completed")
    return 0


if __name__ == "__main__":
    raise SystemExit(run_bridge())
