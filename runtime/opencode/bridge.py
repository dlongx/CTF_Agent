"""Container-side terminal bridge between the CTF platform and OpenCode.

The bridge runs ``opencode run --format json`` inside the task container and
prints readable events to stdout so the existing Go API -> WebSocket -> terminal
pipeline can stay intact.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Any

MAX_SKILL_CHARS = 12000
FLAG_TOKEN_PATTERN = re.compile(r"\b[A-Za-z0-9_-]*flag\{[^`'\"<>\s]+\}", re.IGNORECASE)
SOLVED_FLAG_MARKER = "这道题目已经解出"
WRITEUP_BEGIN_MARKER = "-----BEGIN_CTF_AGENT_WRITEUP-----"
WRITEUP_END_MARKER = "-----END_CTF_AGENT_WRITEUP-----"
WORKSPACE_DIR = Path("/workspace")
MAX_WRITEUP_CHARS = 120000


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
    provider_id: str
    provider_name: str
    provider_npm: str
    base_url: str
    api_key: str
    model: str
    user_hint: str
    session_id: str


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
        provider_id=os.getenv("OPENCODE_PROVIDER_ID", "").strip(),
        provider_name=os.getenv("OPENCODE_PROVIDER_NAME", "CTF Model Gateway").strip(),
        provider_npm=os.getenv("OPENCODE_PROVIDER_NPM", "@ai-sdk/openai-compatible").strip(),
        base_url=os.getenv("OPENCODE_BASE_URL", "").strip().rstrip("/"),
        api_key=os.getenv("OPENCODE_API_KEY", "").strip(),
        model=os.getenv("OPENCODE_MODEL", "").strip(),
        user_hint=os.getenv("CTF_AGENT_USER_HINT", "").strip(),
        session_id=os.getenv("OPENCODE_SESSION_ID", "").strip(),
    )


def parse_skill_ids(raw: str) -> tuple[str, ...]:
    """Parse comma-separated skill ids from the host dispatcher."""
    items = []
    for item in raw.split(","):
        normalized = normalize_category(item)
        if normalized and normalized not in items:
            items.append(normalized)
    return tuple(items)


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
    if (
        config.provider_npm == "@ai-sdk/anthropic"
        and not config.base_url.lower().rstrip("/").endswith("/v1")
    ):
        log(
            "Warning: Anthropic-compatible OPENCODE_BASE_URL usually ends with /v1; "
            f"current value is {config.base_url!r}."
        )

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
    WORKSPACE_DIR.mkdir(parents=True, exist_ok=True)
    (WORKSPACE_DIR / "opencode.json").write_text(
        json.dumps(opencode_config, ensure_ascii=False, indent=2),
        encoding="utf-8",
    )
    log(
        "Thought: wrote OpenCode provider config "
        f"provider={config.provider_id!r} model={config.model!r} base_url={config.base_url!r}."
    )


def sanitize_log_text(text: str, config: BridgeConfig) -> str:
    """Remove sensitive values from OpenCode diagnostics."""
    if config.api_key:
        text = text.replace(config.api_key, "***MASKED***")
    return text


def safe_markdown_stem(name: str, max_chars: int = 120) -> str:
    """Return a filename-safe Markdown stem while preserving readable CTF names."""
    stem = name.strip()
    for char in '<>:"/\\|?*\r\n\t':
        stem = stem.replace(char, "_")
    stem = stem.strip(" .")
    if not stem:
        stem = "writeup"
    if len(stem) > max_chars:
        stem = stem[:max_chars].strip(" .") or "writeup"
    return stem


def writeup_filename(config: BridgeConfig) -> str:
    """Return the fixed writeup filename requested by the platform contract."""
    stem = safe_markdown_stem(config.name)
    if stem.lower().endswith("_wp"):
        return f"{stem}.md"
    stem = safe_markdown_stem(stem, 117)
    return f"{stem}_wp.md"


def writeup_path(config: BridgeConfig) -> Path:
    """Return the only writeup path the bridge will read or create."""
    return WORKSPACE_DIR / writeup_filename(config)


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
    wp_name = writeup_filename(config)
    wp_path = f"/workspace/{wp_name}"

    return (
        "You are solving a CTF challenge inside an isolated Docker container.\n"
        "Use shell and file tools if available. Keep output concise and print the final flag.\n"
        "Only work on the challenge files mounted at /attachments and the provided target.\n"
        "Do not ask the user for interactive confirmation; "
        "solve autonomously within this container.\n"
        "When you summarize the solution, write all narrative explanations in Simplified Chinese. "
        "Keep commands, source code, file names, technical identifiers, formulas, and flags unchanged. "
        "Do not write English writeup prose unless it is part of source code or command output.\n\n"
        "When you solve the challenge, your final response must contain this exact two-line marker block:\n"
        f"{SOLVED_FLAG_MARKER}\n"
        "<exact flag>\n"
        "The marker line must be exactly the Chinese sentence above. The next non-empty line must contain "
        "only the exact flag, with no Markdown, no label, no quotes, and no extra text. The flag wrapper "
        "may be flag{}, DASCTF{}, ISCC{}, or any challenge-specific wrapper.\n\n"
        f"Before the final response, create or overwrite the Markdown writeup file `{wp_path}`. "
        "The writeup must be written in Simplified Chinese, except commands, code, filenames, formulas, "
        "technical identifiers, and flags. Do not paste raw OpenCode logs. Do not include API keys, "
        "environment variables, provider settings, or platform dispatcher logs. The Markdown file should "
        "include these sections: 题目概况, 解题思路, 关键步骤, 关键命令与输出, Flag, 复现步骤, 注意事项. "
        f"The platform will use `{wp_name}` as the final WP download file.\n\n"
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


def build_continuation_prompt(config: BridgeConfig) -> str:
    """Build a concise continuation prompt for an existing OpenCode session."""
    hint = config.user_hint.strip()
    wp_name = writeup_filename(config)
    wp_path = f"/workspace/{wp_name}"
    return (
        "继续同一个CTF题目的解题会话。请根据用户补充信息继续分析，"
        "不要重复已经完成的无关枚举。若本轮解出题目或修正了解法，必须创建或覆盖"
        f"`{wp_path}`，写入结构化中文Markdown WP，包含题目概况、解题思路、关键步骤、"
        "关键命令与输出、Flag、复现步骤和注意事项。不要把原始OpenCode日志当WP。\n\n"
        "找到Flag后必须按以下两行格式收尾:\n"
        f"{SOLVED_FLAG_MARKER}\n"
        "<exact flag>\n\n"
        f"平台最终会保存`{wp_name}`作为WP文件。\n\n"
        "用户补充信息:\n"
        f"{hint}"
    )


def log_delta_preview(text: str) -> None:
    """Print only the newest readable chunk while polling."""
    clean = text.strip()
    if not clean:
        return
    tail = clean[-1200:]
    log("Observation: assistant output updated:")
    log(tail)


def extract_flag_token(text: str) -> str:
    """Return the last flag-looking token found in text."""
    matches = FLAG_TOKEN_PATTERN.findall(text)
    if not matches:
        return ""
    return matches[-1].strip()


def extract_marked_flag(text: str) -> str:
    """Return the first line after the solved marker."""
    lines = text.replace("\r\n", "\n").split("\n")
    for index, line in enumerate(lines):
        if line.strip() != SOLVED_FLAG_MARKER:
            continue
        if marker_looks_instructional(lines, index):
            continue
        for candidate in lines[index + 1 :]:
            clean = candidate.strip().strip("`'\"，。；;")
            if clean:
                return clean
    return ""


def marker_looks_instructional(lines: list[str], index: int) -> bool:
    """Avoid treating prompt examples as solved-marker output."""
    context = "\n".join(lines[max(0, index - 4) : index]).lower()
    markers = (
        "prompt要求",
        "final output contract",
        "flag output contract",
        "strictly output",
        "marker block",
        "最终严格输出",
        "按以下两行",
        "格式收尾",
        "格式要求",
        "示例",
        "example",
        "<exact flag>",
        "<captured-or-last-line>",
    )
    return any(marker in context for marker in markers)


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
        "prompt要求opencode最终严格输出",
        "final output contract",
        "flag output contract",
        "strictly output",
        "marker block",
        "最终严格输出",
        "按以下两行",
        "格式收尾",
        "这道题目已经解出",
    )
    return sum(1 for marker in markers if marker in normalized) >= 2


def extract_session_id(value: Any) -> str:
    """Find an OpenCode session id in a JSON event."""
    if isinstance(value, dict):
        for key in ("sessionID", "sessionId", "session_id"):
            item = value.get(key)
            if isinstance(item, str) and item.strip():
                return item.strip()
        session = value.get("session")
        if isinstance(session, dict):
            item = session.get("id")
            if isinstance(item, str) and item.strip():
                return item.strip()
        for item in value.values():
            found = extract_session_id(item)
            if found:
                return found
    elif isinstance(value, list):
        for item in value:
            found = extract_session_id(item)
            if found:
                return found
    return ""


def should_keep_event_text(text: str) -> bool:
    """Filter obvious metadata strings from terminal JSON events."""
    clean = text.strip()
    if not clean or looks_like_prompt_echo(clean):
        return False
    lower = clean.lower()
    if lower in {
        "assistant",
        "user",
        "system",
        "tool",
        "text",
        "step-start",
        "step-finish",
        "tool-calls",
        "completed",
        "running",
        "pending",
        "build",
        "primary",
    }:
        return False
    if re.fullmatch(r"(msg|prt|ses|toolu|call)_[A-Za-z0-9_-]+", clean):
        return False
    if re.fullmatch(r"[A-Za-z0-9_-]{18,}", clean) and not any(ch.isspace() for ch in clean):
        return False
    return True


def append_terminal_event_text(value: Any, blocks: list[str], inherited_role: str | None = None) -> None:
    """Extract readable text from a single opencode run JSON event."""
    ignored_roles = {"user", "system"}
    if isinstance(value, dict):
        role_value = value.get("role") or value.get("speaker")
        role = str(role_value).strip().lower() if isinstance(role_value, str) else inherited_role
        for key in ("text", "content", "message", "output", "result", "delta", "error"):
            item = value.get(key)
            if isinstance(item, str) and role not in ignored_roles and should_keep_event_text(item):
                blocks.append(item.strip())
        for item in value.values():
            append_terminal_event_text(item, blocks, role)
    elif isinstance(value, list):
        for item in value:
            append_terminal_event_text(item, blocks, inherited_role)


def parse_terminal_event(raw: str, config: BridgeConfig) -> tuple[str, str]:
    """Parse one opencode run output line into session id and readable text."""
    clean = sanitize_log_text(raw.strip(), config)
    if not clean:
        return "", ""
    try:
        event = json.loads(clean)
    except json.JSONDecodeError:
        return "", clean
    blocks: list[str] = []
    append_terminal_event_text(event, blocks)
    return extract_session_id(event), "\n".join(dedupe_preserve_order(blocks))


def dedupe_preserve_order(values: list[str]) -> list[str]:
    """Deduplicate strings without reordering."""
    seen: set[str] = set()
    result: list[str] = []
    for value in values:
        if value in seen:
            continue
        seen.add(value)
        result.append(value)
    return result


def run_opencode_terminal(config: BridgeConfig) -> str:
    """Run OpenCode as a pure terminal command and stream readable JSON events."""
    workspace = WORKSPACE_DIR
    workspace.mkdir(parents=True, exist_ok=True)
    runtime_tmp = Path("/workspace/.tmp")
    runtime_tmp.mkdir(parents=True, exist_ok=True)
    os.environ["TMPDIR"] = str(runtime_tmp)
    prompt = build_continuation_prompt(config) if config.session_id else build_prompt(config)
    args = [
        "opencode",
        "run",
        "--format",
        "json",
        "--model",
        f"{config.provider_id}/{config.model}",
        "--title",
        config.name,
    ]
    if config.session_id:
        args.extend(["--session", config.session_id])
    args.append(prompt)
    log(
        "Action: run OpenCode terminal "
        f"model={config.provider_id}/{config.model} session={config.session_id or 'new'}"
    )
    process = subprocess.Popen(
        args,
        cwd=str(workspace),
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        bufsize=1,
    )
    blocks: list[str] = []
    seen_session = config.session_id
    last_preview = ""
    assert process.stdout is not None
    for raw_line in process.stdout:
        session_id, text = parse_terminal_event(raw_line, config)
        if session_id and session_id != seen_session:
            seen_session = session_id
            log(f"Observation: OpenCode session={session_id}")
        if not text:
            continue
        if text != last_preview:
            log_delta_preview(text)
            last_preview = text
        blocks.append(text)
    exit_code = process.wait()
    final_text = "\n\n".join(dedupe_preserve_order(blocks)).strip()
    if exit_code != 0:
        if final_text and (extract_marked_flag(final_text) or extract_flag_token(final_text)):
            return final_text
        raise RuntimeError(f"opencode run exited with status {exit_code}")
    return final_text


def sanitize_writeup_content(text: str, config: BridgeConfig) -> str:
    """Normalize generated writeup text before emitting it through logs."""
    text = sanitize_log_text(text, config)
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    text = text.replace(WRITEUP_BEGIN_MARKER, "[removed writeup begin marker]")
    text = text.replace(WRITEUP_END_MARKER, "[removed writeup end marker]")
    text = text.strip()
    if len(text) > MAX_WRITEUP_CHARS:
        text = text[:MAX_WRITEUP_CHARS].rstrip()
        text += "\n\n> WP内容超过平台限制，后续内容已截断。"
    return text


def build_fallback_writeup(config: BridgeConfig, final_text: str) -> str:
    """Build a minimal Markdown writeup if OpenCode forgot to create the file."""
    flag = extract_marked_flag(final_text) or extract_flag_token(final_text) or "未捕获"
    excerpt = sanitize_writeup_content(final_text[-6000:], config) or "OpenCode没有输出可用的解题过程。"
    return (
        f"# {config.name}\n\n"
        "## 题目概况\n\n"
        f"- 题型:{config.category}\n"
        f"- 目标:{config.target_ip or '未提供'}\n"
        f"- Flag:{flag}\n\n"
        "## 解题过程\n\n"
        "OpenCode未按要求写入结构化WP文件，平台根据最终可读输出生成了兜底WP。\n\n"
        "## 关键输出\n\n"
        "```text\n"
        f"{excerpt}\n"
        "```\n\n"
        "## 复现步骤\n\n"
        "请参考上方关键输出中的命令、脚本和推导过程复现。\n"
    )


def read_or_create_writeup(config: BridgeConfig, final_text: str) -> str:
    """Read OpenCode's generated writeup or create a fallback file."""
    path = writeup_path(config)
    try:
        content = path.read_text(encoding="utf-8")
    except OSError:
        content = ""
    content = sanitize_writeup_content(content, config)
    if content:
        return content
    content = build_fallback_writeup(config, final_text)
    try:
        path.write_text(content + "\n", encoding="utf-8")
    except OSError as exc:
        log(f"Warning: failed to write fallback writeup {path.name}: {exc}")
    return content


def emit_writeup_output(config: BridgeConfig, final_text: str) -> None:
    """Emit the generated writeup in a stable block for the Go backend."""
    content = read_or_create_writeup(config, final_text)
    log(f"Observation: OpenCode writeup file: {writeup_filename(config)}")
    log(WRITEUP_BEGIN_MARKER)
    log(content)
    log(WRITEUP_END_MARKER)


def emit_final_output(config: BridgeConfig, final_text: str) -> None:
    """Print the final readable output and the marker-based capture block."""
    emit_writeup_output(config, final_text)
    log("Observation: final readable OpenCode output:")
    log(final_text[-12000:])
    final_line = extract_marked_flag(final_text) or extract_flag_token(final_text)
    if final_line:
        log(SOLVED_FLAG_MARKER)
        log(final_line)


def run_bridge() -> int:
    """Run one OpenCode-backed challenge attempt."""
    config = read_config()
    log("\x1b[36m[agent] Phase 5 OpenCode bridge started\x1b[0m")
    log(f"Thought: challenge={config.name!r} category={config.category!r}")
    try:
        configure_opencode(config)
        final_text = run_opencode_terminal(config)
        if not final_text:
            raise RuntimeError("OpenCode terminal finished without readable output")
        emit_final_output(config, final_text)
    except Exception as exc:
        log(f"Final: opencode bridge failed: {exc}")
        return 1
    log("Final: opencode bridge completed")
    return 0


if __name__ == "__main__":
    raise SystemExit(run_bridge())
