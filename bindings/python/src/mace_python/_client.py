from __future__ import annotations

import json as json_module
import subprocess
import tempfile
from pathlib import Path
from typing import Any


class MaceError(RuntimeError):
    def __init__(self, message: str, exit_code: int = 1) -> None:
        super().__init__(message)
        self.exit_code = exit_code


def json(path: str, inject: str | None = None, mace_path: str = "mace", cwd: str | None = None) -> Any:
    output = _run_mace(_json_args(path, inject), mace_path=mace_path, cwd=cwd)
    return json_module.loads(output)


def json_text(path: str, inject: str | None = None, mace_path: str = "mace", cwd: str | None = None) -> str:
    return _run_mace(_json_args(path, inject), mace_path=mace_path, cwd=cwd)


def source(path: str, mace_path: str = "mace", cwd: str | None = None) -> str:
    return _run_mace(["source", path], mace_path=mace_path, cwd=cwd)


def nodes(path: str, mace_path: str = "mace", cwd: str | None = None) -> str:
    return _run_mace(["nodes", path], mace_path=mace_path, cwd=cwd)


def import_json(input_text: str, mace_path: str = "mace", cwd: str | None = None) -> str:
    return _import_text("input.json", input_text, mace_path=mace_path, cwd=cwd)


def import_yaml(input_text: str, mace_path: str = "mace", cwd: str | None = None) -> str:
    return _import_text("input.yaml", input_text, mace_path=mace_path, cwd=cwd)


def import_toml(input_text: str, mace_path: str = "mace", cwd: str | None = None) -> str:
    return _import_text("input.toml", input_text, mace_path=mace_path, cwd=cwd)


def import_file(path: str, mace_path: str = "mace", cwd: str | None = None) -> str:
    with tempfile.TemporaryDirectory(prefix="mace-python-import-") as directory:
        output = _run_mace(["import", path, "--output-dir", directory], mace_path=mace_path, cwd=cwd)
        output_path = next((line for line in output.splitlines() if line.endswith(".mace")), None)
        if output_path is None:
            raise MaceError("import did not report an output file")

        return Path(output_path).read_text(encoding="utf-8")


def _import_text(name: str, input_text: str, mace_path: str, cwd: str | None) -> str:
    with tempfile.TemporaryDirectory(prefix="mace-python-") as directory:
        path = Path(directory) / name
        path.write_text(input_text, encoding="utf-8")
        return import_file(str(path), mace_path=mace_path, cwd=cwd)


def _json_args(path: str, inject: str | None) -> list[str]:
    args = ["json", path]
    if inject is not None:
        args.extend(["--inject", inject])
    return args


def _run_mace(args: list[str], mace_path: str, cwd: str | None) -> str:
    completed = subprocess.run(
        [mace_path, *args],
        cwd=cwd,
        capture_output=True,
        text=True,
        check=False,
    )
    if completed.returncode != 0:
        raise MaceError(completed.stderr.strip() or f"mace exited with code {completed.returncode}", completed.returncode)

    return completed.stdout.strip()
