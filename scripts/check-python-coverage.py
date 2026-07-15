from __future__ import annotations

import dis
import pathlib
import sys
import trace
import types
import unittest


ROOT = pathlib.Path(__file__).resolve().parent.parent
RUNTIME = ROOT / "runtime" / "opencode"
sys.path.insert(0, str(RUNTIME))

import bridge  # noqa: E402


CORE_FUNCTIONS = (
    "parse_duration_seconds",
    "parse_skill_ids",
    "sanitize_log_text",
    "category_guidance",
    "normalize_category",
    "safe_filename_stem",
    "writeup_filename",
    "skill_ids_for_config",
    "read_skill_text",
    "looks_like_prompt_echo",
    "extract_session_id",
    "should_keep_event_text",
    "append_terminal_event_text",
    "parse_terminal_event",
    "dedupe_preserve_order",
    "terminate_process",
    "run_opencode_once",
    "run_opencode_terminal",
    "extract_solved_flag",
)
MINIMUM_PERCENT = 80.0


def executable_lines(code: types.CodeType) -> set[int]:
    lines = {line for _, line in dis.findlinestarts(code) if line != code.co_firstlineno}
    for value in code.co_consts:
        if isinstance(value, types.CodeType):
            lines.update(executable_lines(value))
    return lines


def main() -> int:
    suite = unittest.defaultTestLoader.discover(str(RUNTIME), "test_*.py")
    tracer = trace.Trace(count=1, trace=0, ignoredirs=[sys.prefix, sys.base_prefix])
    result = tracer.runfunc(unittest.TextTestRunner(verbosity=1).run, suite)
    if not result.wasSuccessful():
        return 1

    expected: set[int] = set()
    for name in CORE_FUNCTIONS:
        expected.update(executable_lines(getattr(bridge, name).__code__))
    bridge_path = (RUNTIME / "bridge.py").resolve()
    covered = {
        line
        for (filename, line), count in tracer.results().counts.items()
        if pathlib.Path(filename).resolve() == bridge_path and line in expected and count > 0
    }
    percent = 100.0 * len(covered) / len(expected)
    print(
        f"Bridge core coverage: {percent:.1f}% "
        f"({len(covered)}/{len(expected)} executable lines)"
    )
    if percent < MINIMUM_PERCENT:
        print(f"Required bridge core coverage: {MINIMUM_PERCENT:.1f}%", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
