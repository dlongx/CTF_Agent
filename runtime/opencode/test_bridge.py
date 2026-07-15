from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import bridge


def make_config(root: Path, **overrides: object) -> bridge.BridgeConfig:
    values: dict[str, object] = {
        "name": "demo challenge",
        "category": "misc",
        "description": "read the attachment",
        "target_ip": "",
        "attachment_dir": root / "attachments",
        "skills_dir": root / "skills",
        "skill_ids": ("misc",),
        "provider_id": "ctf",
        "provider_name": "CTF Gateway",
        "provider_npm": "@ai-sdk/openai-compatible",
        "base_url": "https://gateway.example/v1",
        "api_key": "secret-test-key",
        "model": "test-model",
        "user_hint": "",
        "session_id": "",
        "exec_dir": root / "workspace" / ".tmp",
        "run_timeout_seconds": 1200.0,
        "idle_timeout_seconds": 300.0,
    }
    values.update(overrides)
    return bridge.BridgeConfig(**values)


class BridgeTests(unittest.TestCase):
    def test_parse_duration_seconds_supports_go_duration(self) -> None:
        self.assertEqual(bridge.parse_duration_seconds("20m0s", 1), 1200)
        self.assertEqual(bridge.parse_duration_seconds("1h5m30s", 1), 3930)
        self.assertEqual(bridge.parse_duration_seconds("invalid", 42), 42)
        self.assertEqual(bridge.parse_duration_seconds("0s", 42), 42)

    def test_configure_opencode_keeps_key_out_of_files_and_inline_json(self) -> None:
        with tempfile.TemporaryDirectory() as raw_root:
            root = Path(raw_root)
            workspace = root / "workspace"
            workspace.mkdir()
            legacy = workspace / "opencode.json"
            legacy.write_text('{"apiKey":"old"}', encoding="utf-8")
            config = make_config(root)
            with mock.patch.object(bridge, "WORKSPACE_DIR", workspace), mock.patch.dict(
                os.environ, {}, clear=False
            ):
                bridge.configure_opencode(config)
                inline = os.environ["OPENCODE_CONFIG_CONTENT"]
                parsed = json.loads(inline)
                api_key = parsed["provider"]["ctf"]["options"]["apiKey"]
                self.assertEqual(api_key, "{env:OPENCODE_API_KEY}")
                self.assertNotIn(config.api_key, inline)
                self.assertFalse(legacy.exists())

    def test_skill_loader_uses_native_layout_without_truncation(self) -> None:
        with tempfile.TemporaryDirectory() as raw_root:
            root = Path(raw_root)
            skill = root / "skills" / "misc" / "SKILL.md"
            skill.parent.mkdir(parents=True)
            content = "# Misc\n" + ("A" * 20_000)
            skill.write_text(content, encoding="utf-8")
            loaded = bridge.read_skill_text(make_config(root))
            self.assertIn(content, loaded)
            self.assertNotIn("skill truncated", loaded)

    def test_prompt_references_attachments_and_writeup(self) -> None:
        with tempfile.TemporaryDirectory() as raw_root:
            root = Path(raw_root)
            attachment = root / "attachments" / "flag.txt"
            attachment.parent.mkdir(parents=True)
            attachment.write_text("flag{demo}", encoding="utf-8")
            skill = root / "skills" / "misc" / "SKILL.md"
            skill.parent.mkdir(parents=True)
            skill.write_text("# Misc", encoding="utf-8")
            config = make_config(root)
            prompt = bridge.build_prompt(config)
            self.assertIn(str(attachment), prompt)
            self.assertIn("demo-challenge-wp.md", prompt)
            self.assertIn(bridge.SOLVED_MARKER, prompt)

    def test_terminal_event_parsing_extracts_session_and_masks_key(self) -> None:
        with tempfile.TemporaryDirectory() as raw_root:
            config = make_config(Path(raw_root))
            raw = json.dumps(
                {
                    "sessionID": "ses_demo",
                    "role": "assistant",
                    "content": f"result {config.api_key}",
                }
            )
            session_id, text = bridge.parse_terminal_event(raw, config)
            self.assertEqual(session_id, "ses_demo")
            self.assertEqual(text, "result ***MASKED***")

    def test_flag_and_filename_parsers(self) -> None:
        output = f"analysis\n{bridge.SOLVED_MARKER}\nflag{{ok}}\n"
        self.assertEqual(bridge.extract_solved_flag(output), "flag{ok}")
        self.assertEqual(bridge.safe_filename_stem(" a/b:c "), "a-b-c")
        self.assertEqual(bridge.dedupe_preserve_order(["a", "b", "a"]), ["a", "b"])

    def test_read_config_and_category_helpers(self) -> None:
        values = {
            "CHALLENGE_NAME": "name",
            "CHALLENGE_TYPE": "Web-Exploit",
            "CHALLENGE_DESC": "description",
            "TARGET_IP": "127.0.0.1",
            "ATTACHMENT_DIR": "/tmp/attachments",
            "CTF_AGENT_SKILLS_DIR": "/tmp/skills",
            "CTF_AGENT_SKILL_IDS": "web, misc,web",
            "OPENCODE_PROVIDER_ID": "ctf",
            "OPENCODE_PROVIDER_NAME": "Gateway",
            "OPENCODE_PROVIDER_NPM": "npm",
            "OPENCODE_BASE_URL": "https://example.test/v1/",
            "OPENCODE_API_KEY": "key",
            "OPENCODE_MODEL": "model",
            "CTF_AGENT_USER_HINT": "hint",
            "OPENCODE_SESSION_ID": "ses_1",
            "CTF_AGENT_EXEC_DIR": "/tmp/exec",
            "CTF_AGENT_OPENCODE_RUN_TIMEOUT": "1m30s",
            "CTF_AGENT_OPENCODE_IDLE_TIMEOUT": "2.5s",
        }
        with mock.patch.dict(os.environ, values, clear=True):
            config = bridge.read_config()
        self.assertEqual(config.name, "name")
        self.assertEqual(config.base_url, "https://example.test/v1")
        self.assertEqual(config.skill_ids, ("web", "misc"))
        self.assertEqual(config.run_timeout_seconds, 90)
        self.assertEqual(config.idle_timeout_seconds, 2.5)
        self.assertEqual(bridge.normalize_category(" Web-Exploit "), "web_exploit")
        self.assertEqual(bridge.parse_skill_ids("PWN,pwn, crypto"), ("pwn", "crypto"))
        for category in ("web", "pwn", "crypto", "reverse", "forensics", "misc", "unknown"):
            self.assertTrue(bridge.category_guidance(category))

    def test_configure_opencode_rejects_incomplete_credentials(self) -> None:
        with tempfile.TemporaryDirectory() as raw_root:
            root = Path(raw_root)
            workspace = root / "workspace"
            cases = [
                make_config(root, provider_id="", model=""),
                make_config(root, provider_id="", model="model"),
                make_config(root, base_url=""),
                make_config(root, api_key=""),
            ]
            with mock.patch.object(bridge, "WORKSPACE_DIR", workspace):
                for config in cases:
                    with self.subTest(config=config):
                        with self.assertRaises(RuntimeError):
                            bridge.configure_opencode(config)

    def test_skill_fallback_and_prompt_variants(self) -> None:
        with tempfile.TemporaryDirectory() as raw_root:
            root = Path(raw_root)
            skill = root / "skills" / "misc.md"
            skill.parent.mkdir(parents=True)
            skill.write_text("# Legacy misc", encoding="utf-8")
            config = make_config(
                root,
                name="***",
                target_ip="10.0.0.1",
                user_hint="try harder",
                session_id="ses_old",
            )
            self.assertIn("Legacy misc", bridge.read_skill_text(config))
            self.assertEqual(bridge.writeup_filename(config), "challenge-wp.md")
            continuation = bridge.build_continuation_prompt(config)
            recovery = bridge.build_session_recovery_prompt(config)
            self.assertIn("try harder", continuation)
            self.assertIn("新的OpenCode session", recovery)
            self.assertIn(
                "Skill file was not available",
                bridge.read_skill_text(make_config(root, skills_dir=root / "missing")),
            )

    def test_event_filters_and_nested_parser(self) -> None:
        with tempfile.TemporaryDirectory() as raw_root:
            config = make_config(Path(raw_root))
            self.assertEqual(bridge.parse_terminal_event("", config), ("", ""))
            self.assertEqual(bridge.parse_terminal_event("plain text", config), ("", "plain text"))
            nested = {
                "session": {"id": "ses_nested"},
                "parts": [
                    {"role": "user", "text": "hidden user prompt"},
                    {"role": "assistant", "text": "useful"},
                    {"role": "assistant", "content": "useful"},
                    {"role": "assistant", "output": "completed"},
                ],
            }
            session, text = bridge.parse_terminal_event(json.dumps(nested), config)
            self.assertEqual(session, "ses_nested")
            self.assertEqual(text, "useful")
            self.assertEqual(bridge.extract_session_id([{"session_id": "ses_list"}]), "ses_list")
            self.assertEqual(bridge.extract_session_id("none"), "")
            for value in ("", "assistant", "msg_123456789012345678", "abcdefghijklmnopqr"):
                self.assertFalse(bridge.should_keep_event_text(value))
            self.assertTrue(bridge.should_keep_event_text("verified output"))
            self.assertTrue(
                bridge.looks_like_prompt_echo(
                    "Active CTF Skills: misc\nAttachments are mounted read-only at /attachments"
                )
            )

    def test_run_opencode_once_streams_and_cleans_prompt(self) -> None:
        with tempfile.TemporaryDirectory() as raw_root:
            root = Path(raw_root)
            workspace = root / "workspace"
            config = make_config(root, exec_dir=workspace / ".tmp")
            original_popen = subprocess.Popen
            lines = [
                json.dumps({"sessionID": "ses_stream", "role": "assistant", "text": "step one"}),
                json.dumps({"role": "assistant", "text": "step two"}),
            ]
            program = "import sys;sys.stdout.write(" + repr("\n".join(lines)) + ");sys.stdout.flush()"

            def fake_popen(_args: object, **kwargs: object) -> subprocess.Popen[bytes]:
                return original_popen([sys.executable, "-c", program], **kwargs)

            with mock.patch.object(bridge, "WORKSPACE_DIR", workspace), mock.patch.object(
                bridge.subprocess, "Popen", side_effect=fake_popen
            ), mock.patch.object(bridge, "log"):
                text, session = bridge.run_opencode_once(config, "secret prompt", "")
            self.assertEqual(session, "ses_stream")
            self.assertIn("step one", text)
            self.assertIn("step two", text)
            self.assertFalse((config.exec_dir / "ctf-agent-prompt.md").exists())

    def test_run_opencode_once_enforces_idle_timeout(self) -> None:
        with tempfile.TemporaryDirectory() as raw_root:
            root = Path(raw_root)
            workspace = root / "workspace"
            config = make_config(
                root,
                exec_dir=workspace / ".tmp",
                run_timeout_seconds=5,
                idle_timeout_seconds=0.05,
            )
            original_popen = subprocess.Popen

            def fake_popen(_args: object, **kwargs: object) -> subprocess.Popen[bytes]:
                return original_popen(
                    [sys.executable, "-c", "import time; time.sleep(10)"], **kwargs
                )

            with mock.patch.object(bridge, "WORKSPACE_DIR", workspace), mock.patch.object(
                bridge.subprocess, "Popen", side_effect=fake_popen
            ), mock.patch.object(bridge, "log"):
                with self.assertRaisesRegex(RuntimeError, "idle timeout"):
                    bridge.run_opencode_once(config, "secret prompt", "")
            self.assertFalse((config.exec_dir / "ctf-agent-prompt.md").exists())

    def test_terminal_recovery_writeup_and_bridge_result(self) -> None:
        with tempfile.TemporaryDirectory() as raw_root:
            root = Path(raw_root)
            workspace = root / "workspace"
            workspace.mkdir()
            config = make_config(root, session_id="ses_old")
            with mock.patch.object(
                bridge, "run_opencode_once", side_effect=[("", "ses_old"), ("recovered", "ses_new")]
            ) as run_once, mock.patch.object(bridge, "log"):
                self.assertEqual(bridge.run_opencode_terminal(config), ("recovered", "ses_new"))
                self.assertEqual(run_once.call_count, 2)

            final = f"done\n{bridge.SOLVED_MARKER}\nflag{{wp}}"
            wp_name = bridge.writeup_filename(config)

            def create_writeup(*_args: object) -> tuple[str, str]:
                (workspace / wp_name).write_text("# WP", encoding="utf-8")
                return "", "ses_old"

            with mock.patch.object(bridge, "WORKSPACE_DIR", workspace), mock.patch.object(
                bridge, "run_opencode_once", side_effect=create_writeup
            ), mock.patch.object(bridge, "log"):
                self.assertEqual(bridge.ensure_writeup(config, final, "ses_old"), wp_name)
                self.assertEqual(bridge.ensure_writeup(config, "not solved", "ses_old"), "")

            with mock.patch.object(bridge, "read_config", return_value=config), mock.patch.object(
                bridge, "configure_opencode"
            ), mock.patch.object(
                bridge, "run_opencode_terminal", return_value=(final, "ses_new")
            ), mock.patch.object(bridge, "ensure_writeup"), mock.patch.object(
                bridge, "emit_final_output"
            ), mock.patch.object(bridge, "log"):
                self.assertEqual(bridge.run_bridge(), 0)
            with mock.patch.object(bridge, "read_config", return_value=config), mock.patch.object(
                bridge, "configure_opencode", side_effect=RuntimeError(config.api_key)
            ), mock.patch.object(bridge, "log") as output:
                self.assertEqual(bridge.run_bridge(), 1)
                self.assertNotIn(config.api_key, " ".join(str(call) for call in output.call_args_list))


if __name__ == "__main__":
    unittest.main()
