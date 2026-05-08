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
WORKSPACE_DIR = Path("/workspace")
DEFAULT_EXEC_DIR = WORKSPACE_DIR / ".tmp"
SOLVED_MARKER = "这道题目已经解出"


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
    exec_dir: Path


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
        exec_dir=Path(os.getenv("CTF_AGENT_EXEC_DIR", str(DEFAULT_EXEC_DIR))),
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


def safe_filename_stem(value: str) -> str:
    """Return a conservative workspace filename stem."""
    value = value.strip()
    if not value:
        return ""
    chars: list[str] = []
    last_dash = False
    for char in value:
        if (
            "a" <= char <= "z"
            or "A" <= char <= "Z"
            or "0" <= char <= "9"
            or "\u4e00" <= char <= "\u9fff"
            or char in "-_."
        ):
            chars.append(char)
            last_dash = False
        else:
            if not last_dash:
                chars.append("-")
                last_dash = True
    stem = "".join(chars).strip("-_. ")
    return stem[:80].strip("-_. ")


def writeup_filename(config: BridgeConfig) -> str:
    """Return the expected writeup filename inside /workspace."""
    stem = safe_filename_stem(config.name) or "challenge"
    return f"{stem}-wp.md"


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
    exec_dir = str(config.exec_dir)
    wp_name = writeup_filename(config)

    return (
        "You are solving a CTF challenge inside an isolated Docker container.\n"
        "Use shell and file tools if available. Keep output concise and preserve useful command output.\n"
        "Only work on the challenge files mounted at /attachments and the provided target.\n"
        "Do not ask the user for interactive confirmation; "
        "solve autonomously within this container.\n"
        f"Use `{exec_dir}` for temporary scripts, compiled binaries, brute-force helpers, "
        "and any file that must be executed. Do not execute generated programs from `/tmp`; "
        "`/tmp` is mounted noexec by the platform.\n"
        "For network checks, prefer curl, nc, or nmap -sT before raw-socket tools such as ping "
        "or SYN scans, because container capabilities are intentionally restricted.\n"
        "When you summarize the solution, write all narrative explanations in Simplified Chinese. "
        "Keep commands, source code, file names, technical identifiers, formulas, and flags unchanged. "
        "Do not write English writeup prose unless it is part of source code or command output.\n\n"
        "The host platform will keep running you until you explicitly report that the challenge is solved. "
        "Do not output the solved marker until you have verified the final flag.\n"
        "When solved, your final answer must contain these two consecutive lines exactly:\n"
        f"{SOLVED_MARKER}\n"
        "<the complete flag on this entire line>\n"
        "The flag format is not fixed. Do not wrap it in quotes unless the quote characters are part of the flag.\n"
        f"After finding the flag, write a Simplified Chinese writeup to `/workspace/{wp_name}`. "
        "The writeup must include the key reasoning, commands, relevant outputs, and the final flag.\n\n"
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
    exec_dir = str(config.exec_dir)
    wp_name = writeup_filename(config)
    return (
        "继续同一个CTF题目的解题会话。请根据用户补充信息继续分析，"
        "不要重复已经完成的无关枚举。宿主平台会一直续跑直到你按协议声明解出；"
        "未确认最终Flag前不要输出解出标记。\n\n"
        f"需要生成并执行脚本或二进制时，统一放在`{exec_dir}`或`/workspace`下，"
        "不要从`/tmp`执行，因为平台将`/tmp`挂载为noexec。\n"
        "网络连通性检查优先使用curl、nc或nmap -sT，避免依赖raw socket权限。\n\n"
        "确认解出后，最终回答必须包含连续两行：\n"
        f"{SOLVED_MARKER}\n"
        "<下一整行输出完整Flag>\n"
        f"同时把简体中文解题过程写入`/workspace/{wp_name}`，保留命令、输出和Flag原样。\n\n"
        "用户补充信息:\n"
        f"{hint}"
    )


def build_session_recovery_prompt(config: BridgeConfig) -> str:
    """Build a recovery prompt when a retained OpenCode session returns no text."""
    hint = config.user_hint.strip()
    exec_dir = str(config.exec_dir)
    wp_name = writeup_filename(config)
    return (
        "这是同一个CTF题目的恢复执行。上一轮OpenCode session没有返回可读输出，"
        "所以平台改用新的OpenCode session继续。不要把这当成新题重头写总结。\n\n"
        "先检查`/workspace`和"
        f"`{exec_dir}`中已有的脚本、临时文件和输出，"
        "从已有线索继续做具体验证。不要重复已经完成的大范围无关枚举。\n\n"
        "未确认最终Flag前不要输出解出标记。确认解出后，最终回答必须包含连续两行：\n"
        f"{SOLVED_MARKER}\n"
        "<下一整行输出完整Flag>\n"
        f"并写入`/workspace/{wp_name}`作为简体中文WP。\n\n"
        f"Name: {config.name}\n"
        f"Category: {config.category}\n"
        f"Target IP: {config.target_ip or 'not provided'}\n"
        f"Description: {config.description}\n"
        f"Category guidance: {category_guidance(config.category)}\n"
        "用户补充信息:\n"
        f"{hint or '继续上一轮未完成的验证。'}"
    )


def log_delta_preview(text: str) -> None:
    """Print only the newest readable chunk while polling."""
    clean = text.strip()
    if not clean:
        return
    tail = clean[-1200:]
    log("Observation: assistant output updated:")
    log(tail)


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


def run_opencode_once(config: BridgeConfig, prompt: str, session_id: str) -> str:
    """Run one OpenCode terminal command and return accumulated readable text."""
    workspace = WORKSPACE_DIR
    workspace.mkdir(parents=True, exist_ok=True)
    runtime_tmp = config.exec_dir
    runtime_tmp.mkdir(parents=True, exist_ok=True)
    os.environ["TMPDIR"] = str(runtime_tmp)
    os.environ["CTF_AGENT_EXEC_DIR"] = str(runtime_tmp)
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
    if session_id:
        args.extend(["--session", session_id])
    args.append(prompt)
    log(
        "Action: run OpenCode terminal "
        f"model={config.provider_id}/{config.model} session={session_id or 'new'}"
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
    seen_session = session_id
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
        raise RuntimeError(f"opencode run exited with status {exit_code}")
    return final_text


def run_opencode_terminal(config: BridgeConfig) -> str:
    """Run OpenCode and recover once if an existing session returns no text."""
    prompt = build_continuation_prompt(config) if config.session_id else build_prompt(config)
    final_text = run_opencode_once(config, prompt, config.session_id)
    if final_text or not config.session_id:
        return final_text
    log(
        "Observation: OpenCode session returned no readable output; "
        "retrying with a new recovery session"
    )
    return run_opencode_once(config, build_session_recovery_prompt(config), "")


def emit_final_output(_config: BridgeConfig, final_text: str) -> None:
    """Print the final readable output for the host log viewer."""
    log("Observation: final readable OpenCode output:")
    log(final_text[-12000:])


def extract_solved_flag(final_text: str) -> str:
    """Return the protocol flag line when the model declared the challenge solved."""
    lines = final_text.replace("\r\n", "\n").split("\n")
    for index, line in enumerate(lines):
        if line.strip() != SOLVED_MARKER:
            continue
        if index + 1 >= len(lines):
            return ""
        flag = lines[index + 1].rstrip("\r")
        if flag.strip():
            return flag
    return ""


def ensure_writeup(config: BridgeConfig, final_text: str) -> str:
    """Ask OpenCode once to create the writeup when a solved run did not leave one."""
    if not extract_solved_flag(final_text):
        return ""
    wp_name = writeup_filename(config)
    wp_path = WORKSPACE_DIR / wp_name
    if wp_path.exists() and wp_path.is_file() and wp_path.stat().st_size > 0:
        log(f"Observation: writeup file={wp_name}")
        return wp_name

    prompt = (
        "你已经确认这道CTF题解出。现在只补写WP文件，不要重新解题。\n"
        f"请把完整简体中文解题过程写入`/workspace/{wp_name}`，内容包括关键推理、"
        "执行过的重要命令、关键输出、最终Flag，以及可复现步骤。"
    )
    try:
        run_opencode_once(config, prompt, config.session_id)
    except Exception as exc:
        log(f"Warning: failed to generate writeup: {exc}")
    if wp_path.exists() and wp_path.is_file() and wp_path.stat().st_size > 0:
        log(f"Observation: writeup file={wp_name}")
        return wp_name
    return ""


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
        ensure_writeup(config, final_text)
        emit_final_output(config, final_text)
    except Exception as exc:
        log(f"Final: opencode bridge failed: {exc}")
        return 1
    log("Final: opencode bridge completed")
    return 0


if __name__ == "__main__":
    raise SystemExit(run_bridge())
